# BPF programs

This folder contains bpf programs that are used by ebpf collector
of CEEMS exporter. There are two sets of bpf programs:

- `vfs`: These programs trace VFS events like `vfs_read`, `vfs_write`, _etc._
- `network`: These programs trace network ingress and egress traffic.

## VFS events

Currently, bpf programs trace following events:

- `vfs_write`
- `vfs_read`
- `vfs_create`
- `vfs_open`
- `vfs_unlink`
- `vfs_mkdir`
- `vfs_rmdir`

For `x86` architectures, `fexit` probes are used to trace the functions where as
for other architectures, `kretprobes` are used. The reason is that `fentry/fexit`
probes are not available for all the architectures.

Between different kernel versions from `5.8`, the function signatures of several
`vfs_*` functions have changed. Thus, we compile bpf programs for different kernel
versions and load appropriate program at runtime after detecting the kernel
version of current host.

## Network events

For network events the bpf programs trace following functions:

- `tcp_sendmsg`
- `tcp_sendpage` (when exists. Removed from kernel >= 6.5)
- `tcp_recvmsg`
- `udp_sendmsg`
- `udp_sendpage` (when exists. Removed from kernel >= 6.5)
- `udp_recvmsg`
- `udpv6_sendmsg`
- `udpv6_recvmsg`

By tracing above stated functions, we can get TCP and UDP traffics for both IPv4 and
IPv6 families per cgroup. Function signatures for certain functions have changed in
kernel 5.19 and it is taken into account. Based on the kernel version at the runtime,
appropriate object file that contains correct bpf programs are loaded.

A first attempt was made to monitor the network traffic by tracing more low-level
functions: `__netif_receive_skb_core` for ingress traffic and
`__dev_queue_xmit` for egress traffic. `__netif_receive_skb_core` is the core function
where packet processing starts when it reaches the NIC. As we send a lot of packets
at once (which happens quite often in real world cases), only few packets are processed
in the user context and the rest of the packets are kept in device queues. Then kernel's
SoftIRQ will handle the packets in queue for each CPU. Since SoftIRQ happens in kernel
space we lose the cgroup context of the user. The consequence is that only few packets will
be attributed to the cgroup and rest are attributed to the system.

The objective of CEEMS is to accurately monitor the network traffic _per cgroup_ and hence,
we resorted to monitor the network on more high level sockets where user context is
preserved.

## Building

`clang >= 18` is a prerequisite to build the bpf programs. The programs can be build
using provided Makefile

```bash
make all
```

This will create different bpf program objects in `objs/` folder. These object files are
embedded into go binary during build process. Appropriate object file based on the current
kernel version will be loaded at runtime.
