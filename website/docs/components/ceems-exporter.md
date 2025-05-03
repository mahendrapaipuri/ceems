---
sidebar_position: 1
---

# CEEMS Exporter

## Background

`ceems_exporter` is the Prometheus exporter that exposes individual compute unit
metrics, RAPL energy, IPMI power consumption, emission factor, GPU to compute unit
mapping, performance metrics, IO and network metrics. Besides, the exporter supports
a [HTTP discovery component](https://grafana.com/docs/alloy/latest/reference/components/discovery/discovery.http/)
that can provide a list of targets to [Grafana Alloy](https://grafana.com/docs/alloy/latest/).

`ceems_exporter` collectors can be categorized as follows:

### Resource manager collectors

These collectors export metrics from different resource managers.

- Slurm collector: Exports SLURM job metrics like CPU, memory and GPU indices to job ID maps
- Libvirt collector: Exports libvirt managed VMs metrics like CPU, memory, IO, _etc_.
- k8s collector: Exports k8s managed pods metrics like CPU, memory, IO, _etc_.

### Energy-Related Collectors

These collectors export energy-related metrics from different
sources on the compute node.

- IPMI collector: Exports power usage reported by `ipmi` tools
- Redfish collector: Exports power usage reported by the [Redfish API](https://www.dmtf.org/standards/redfish)
- Cray PM counter collector: Exports power usage reported by [Cray's PM counters](https://cray-hpe.github.io/docs-csm/en-10/operations/power_management/user_access_to_compute_node_power_data/)
- HWMon collector: Exports power and energy values reported by [HWMON](https://www.kernel.org/doc/Documentation/hwmon/sysfs-interface)
- RAPL collector: Exports RAPL energy metrics

### Emissions-Related Collectors

This collector exports emissions-related metrics that are used
in estimating carbon footprint:

- Emissions collector: Exports emission factors (g eCO2/kWh)

### Node Metrics Collectors

These collectors export node-level metrics:

- CPU collector: Exports CPU time in different modes (at node level)
- Meminfo collector: Exports memory-related statistics (at node level)

### Perf related collectors

In addition to the above-stated collectors, there are common "sub-collectors" that
can be reused with different collectors. These sub-collectors provide auxiliary
metrics like IO, networking, performance _etc_. Currently available sub-collectors are:

- Perf sub-collector: Exports hardware, software, and cache performance metrics
- eBPF sub-collector: Exports IO and network-related metrics
- RDMA sub-collector: Exports selected RDMA stats

These sub-collectors are not meant to work alone and can only be enabled when
a main collector that monitors the resource manager's compute units is activated.

## Sub-collectors

### Perf sub-collector

The Perf sub-collector exports performance-related metrics fetched from Linux's
[perf](https://perf.wiki.kernel.org/index.php/Main_Page) subsystem. Currently,
it supports hardware, software, and hardware cache events. More advanced details
on perf events can be found in
[Brendan Gregg's blogs](https://www.brendangregg.com/perf.html#Events). Currently supported events are listed as follows:

#### Hardware Events

- Total cycles
- Retired instructions
- Cache accesses. Usually this indicates Last Level Cache accesses but this may vary depending on your CPU
- Cache misses.  Usually this indicates Last Level Cache misses; this is intended to be used in conjunction with the PERF_COUNT_HW_CACHE_REFERENCES event to calculate cache miss rates
- Retired branch instructions
- Mis-predicted branch instructions

#### Software Events

- Number of page faults
- Number of context switches
- Number of CPU migrations
- Number of minor page faults (these do not require disk I/O to handle)
- Number of major page faults (these require disk I/O to handle)

#### Hardware Cache Events

- Number of L1 data cache read hits
- Number of L1 data cache read misses
- Number of L1 data cache write hits
- Number of instruction L1 instruction read misses
- Number of instruction TLB read hits
- Number of instruction TLB read misses
- Number of last level read hits
- Number of last level read misses
- Number of last level write hits
- Number of last level write misses
- Number of Branch Prediction Units (BPU) read hits
- Number of Branch Prediction Units (BPU) read misses

### eBPF sub-collector

The eBPF sub-collector uses [eBPF](https://ebpf.io/what-is-ebpf/) to monitor network and
IO statistics. More details on eBPF are outside the scope of the current documentation.
This sub-collector loads various BPF programs that trace several kernel functions that
are relevant to network and IO.

#### IO Metrics

The core concept for gathering IO metrics is based on the Linux kernel Virtual File System
layer. From the [documentation](https://www.kernel.org/doc/html/latest/filesystems/vfs.html), VFS
can be defined as:

> The Virtual File System (also known as the Virtual Filesystem Switch) is the software
layer in the kernel that provides the filesystem interface to userspace programs.
It also provides an abstraction within the kernel which allows different filesystem
implementations to coexist.

Thus, all IO activity must go through the VFS layer. By tracing appropriate functions,
we can monitor IO metrics. At the same time, these VFS kernel functions have process context
readily available, so it is possible to attribute each IO operation to a given cgroup.
By leveraging these two ideas, it is possible to gather IO metrics for each cgroup. The
following functions are traced in this sub-collector:

- `vfs_read`
- `vfs_write`
- `vfs_open`
- `vfs_create`
- `vfs_mkdir`
- `vfs_unlink`
- `vfs_rmdir`

All the above kernel functions are exported and have a fairly stable API. By tracing these
functions, we can monitor:

- Number of read bytes
- Number of write bytes
- Number of read requests
- Number of write requests
- Number of read errors
- Number of write errors
- Number of open requests
- Number of open errors
- Number of create requests
- Number of create errors
- Number of unlink requests
- Number of unlink errors

Read and write statistics are aggregated based on mount points. Most of the production
workloads use high performance network file systems which are mounted on compute nodes
at specific mount points. Different filesystems may offer different QoS, IOPS capabilities and hence,
it is beneficial to expose the IO stats on per mountpoint basis instead of aggregating
statistics from different types of file systems. It is possible to configure CEEMS
exporter to provide a list of mount points to monitor at runtime.

Rest of the metrics are aggregated globally due to complexity in retrieving the mount
point information from kernel function arguments.

:::note[NOTE]

Total aggregate statistics should be very accurate for each cgroup. However, if underlying
file system uses async IO, the IO rate statistics might not reflect true rate as kernel
functions return immediately after submitting IO task to the driver of underlying filesystem.
In the case of sync IO, kernel function blocks until IO operation has finished and thus, we
get accurate rate statistics.

:::

IO data path is highly complex with a lot of caching involved for several filesystem drivers.
The statistics reported by these bpf programs are the ones "observed" by the user's workloads
rather than from the filesystem's perspective. The advantage of this approach is that we can use
these bpf programs to monitor different types of filesystems in a unified manner without having
to support different filesystems separately.

#### Network metrics

The eBPF sub-collector traces kernel functions that monitor the following types of network
events:

- TCP with IPv4 and IPv6
- UDP with IPv4 and IPv6

Most of the production workloads use TCP/UDP for communication and hence, only these
two protocols are supported. This is done by tracing the following kernel functions:

- `tcp_sendmsg`
- `tcp_sendpage` for kernels < 6.5
- `tcp_recvmsg`
- `udp_sendmsg`
- `udp_sendpage` for kernels < 6.5
- `udp_recvmsg`
- `udpv6_sendmsg`
- `udpv6_recvmsg`

The following metrics are provided by tracing above functions. All the metrics are provided
per protocol (TCP/UDP) and per IP family (IPv4/IPv6).

- Number of egress bytes
- Number of egress packets
- Number of ingress bytes
- Number of ingress packets
- Number of retransmission bytes (only for TCP)
- Number of retransmission packets (only for TCP)

### RDMA sub-collector

Data transfer in RDMA happens directly between RDMA NIC and remote machine memory bypassing
CPU. Thus, it is hard to trace RDMA data transfer on a compute unit granularity. However,
the system wide data transfer metrics are readily available at `/sys/class/infiniband`
pseudo-filesystem. Thus, this sub-collector exports important system wide RDMA stats along
with few low-level metrics on a compute unit level.

#### System wide RDMA stats

- Number of data octets received on all links
- Number of data octets transmitted on all links
- Number of packets received on all VLs by this port (including errors)
- Number of packets transmitted on all VLs from this port (including errors)
- Number of packets received on the switch physical port that are discarded
- Number of packets not transmitted from the switch physical port
- Number of inbound packets discarded by the port because the port is down or congested
- Number of outbound packets discarded by the port because the port is down or congested
- Number of packets containing an error that were received on this port
- State of the InfiniBand port

#### Per compute unit RDMA stats

- Number of active Queue Pairs (QPs)
- Number of active Completion Queues (CQs)
- Number of active Memory Regions (MRs)
- Length of active CQs
- Length of active MRs

In the case of Mellanox devices, the following metrics are available for each compute unit:

- Number of received write requests for the associated QPs
- Number of received read requests for the associated QPs
- Number of received atomic request for the associated QPs
- Number of times requester detected CQEs completed with errors
- Number of times requester detected CQEs completed with flushed errors
- Number of times requester detected remote access errors
- Number of times requester detected remote invalid request errors
- Number of times responder detected CQEs completed with errors
- Number of times responder detected CQEs completed with flushed errors
- Number of times responder detected local length errors
- Number of times responder detected remote access errors

In order to interpret these metrics, please take a look at this
[very nice blog](https://cuterwrite.top/en/p/rdma-element/) which explains internals
of RDMA very well.

## Collectors

### Slurm collector

Slurm collector exports the job related metrics like usage of CPU, DRAM, RDMA, _etc_.
This is done by walking through the cgroups created by SLURM daemon on compute node on
every scrape request. As walking through the cgroups pseudo file system is _very cheap_,
this will have zero to negligible impact on the actual job. The exporter has been
heavily inspired by [cgroups_exporter](https://github.com/treydock/cgroup_exporter)
and it supports both cgroups **v1** and **v2**.

:::warning[WARNING]

For SLURM collector to work properly, SLURM needs to be configured well to use all
the available cgroups controllers. At least `cpu` and `memory` controllers must be
enabled, if not cgroups will not contain any accounting information. Without `cpu`
and `memory` accounting information, it is not possible to estimate energy consumption
of the job.

More details on how to configure SLURM to get accounting information from cgroups can
be found in [Configuration](../configuration/resource-managers.md) section.

:::

For jobs with GPUs, we must have the GPU ordinals allocated to
each job so that we can match GPU metrics scraped by either
[dcgm-exporter](https://github.com/NVIDIA/dcgm-exporter) or
[amd-smi-exporter](https://github.com/amd/amd_smi_exporter) to jobs. Unfortunately,
this information is not available post-mortem of the job and hence, the CEEMS exporter
exports a metric that maps the job ID to GPU ordinals.

Currently, the list of job related metrics exported by SLURM exporter are as follows:

- Job current CPU time in user and system mode
- Job CPUs limit (Number of CPUs allocated to the job)
- Job current total memory usage
- Job total memory limit (Memory allocated to the job)
- Job current RSS memory usage
- Job current cache memory usage
- Job current number of memory usage hits limits
- Job current memory and swap usage
- Job current memory and swap usage hits limits
- Job total memory and swap limit
- Job CPU and memory pressures
- Job maximum RDMA HCA handles
- Job maximum RDMA HCA objects
- Job to GPU ordinal mapping (when GPUs found on the compute node)
- Current number of jobs on the compute node

More information on the metrics can be found in kernel documentation of
[cgroups v1](https://www.kernel.org/doc/Documentation/cgroup-v1/memory.txt) and
[cgroups v2](https://git.kernel.org/pub/scm/linux/kernel/git/tj/cgroup.git/tree/Documentation/admin-guide/cgroup-v2.rst).

Slurm collector supports [perf](./ceems-exporter.md#perf-sub-collector)
and [eBPF](./ceems-exporter.md#ebpf-sub-collector) sub-collectors. Hence, in
addition to the above stated metrics, all the metrics available in the sub-collectors
can also be reported for each cgroup.

### Libvirt collector

Similar to slurm collector, libvirt collector exports metrics of VMs managed
by libvirt. This collector is useful monitor Openstack clusters where
[nova](https://docs.openstack.org/nova/latest/) uses libvirt to manage lifecycle
of the VMs. The exported metrics include usage of CPU, DRAM, block IO retrieved
from cgroups. The collector supports both cgroups v1 and v2.

When GPUs are present on the compute node, like in the case of Slurm, we will
need information on which GPU is used by which VM. This information can be
obtained in libvirt's XML file that keeps the state of the VM.

- NVIDIA's MIG instances uses a similar approach to vGPU to expose GPUs inside
guests and hence, similar limitations apply.

Thus, currently it is not possible to reliably monitor the energy and usage
metrics of libvirt instances with GPUs. In any case, the exporter will always
export the GPU UUID to instance UUID to keep track of which instance is using
which GPU. If the above stated limitations are addressed upstream, CEEMS will
allow us to track usage metrics of GPU instances as well.

Currently, the list of metrics exported by Libvirt exporter are as follows:

- Instance current CPU time in user and system mode
- Instance CPUs limit (Number of CPUs allocated to the job)
- Instance current total memory usage
- Instance total memory limit (Memory allocated to the job)
- Instance current RSS memory usage
- Instance current cache memory usage
- Instance current number of memory usage hits limits
- Instance current memory and swap usage
- Instance current memory and swap usage hits limits
- Instance total memory and swap limit
- Instance block IO read and write bytes
- Instance block IO read and write requests
- Instance CPU, memory and IO pressures
- Instance to GPU ordinal mapping (when GPUs found on the compute node)
- Current number of instances on the compute node

Similar to Slurm, libvirt exporter supports
[perf](./ceems-exporter.md#perf-sub-collector)
and [eBPF](./ceems-exporter.md#ebpf-sub-collector) sub-collectors.

:::warning[WARNING]

Libvirt will have no information about the guest running inside the
cgroup and hence, it is not possible to profile individual processes
inside the guest. Therefore, metrics exported by
[perf](./ceems-exporter.md#perf-sub-collector) are for entire VM and
it is not possible to have more fine grained control on which processes
inside the guest can be profiled.

:::

### k8s collector

[Kubelet](https://kubernetes.io/docs/reference/command-line-tools-reference/kubelet/)
manages the creation and deletion of k8s pods and the current
exporter is capable of exporting metrics of each pod. Currently Kubelet supports
two different types of cgroup drivers: `cgroupfs` and `systemd` and the collector
supports both of them for cgroups v1 and v2.

When GPUs are present on the compute node, the collector finds the devices that are
attached to pods by querying Kubelet using
[Pod Resources API](https://kubernetes.io/blog/2023/08/23/kubelet-podresources-api-ga/).

Currently, the list of metrics exported by Libvirt exporter are as follows:

- Pod current CPU time in user and system mode
- Pod CPUs limit (Number of CPUs allocated to the job)
- Pod current total memory usage
- Pod total memory limit (Memory allocated to the job)
- Pod current RSS memory usage
- Pod current cache memory usage
- Pod current number of memory usage hits limits
- Pod current memory and swap usage
- Pod current memory and swap usage hits limits
- Pod total memory and swap limit
- Pod block IO read and write bytes
- Pod block IO read and write requests
- Pod CPU, memory and IO pressures
- Pod to GPU ordinal mapping (when GPUs found on the compute node)
- Current number of pods on the compute node

Similar to Slurm and libvirt collectors, k8s collector supports
[perf](./ceems-exporter.md#perf-sub-collector)
and [eBPF](./ceems-exporter.md#ebpf-sub-collector) sub-collectors.

### IPMI collector

The IPMI collector reports the current power usage by the node reported by
[IPMI DCMI](https://www.intel.com/content/dam/www/public/us/en/documents/technical-specifications/dcmi-v1-5-rev-spec.pdf)
command specification. Generally IPMI DCMI is available on all types of nodes and
manufacturers as it is needed for BMC control. There are several IPMI implementation
available like FreeIPMI, OpenIPMI, IPMIUtil, _etc._ As IPMI DCMI specification is
standardized, different implementations must report the same power usage value of the node.

Currently, the metrics exposed by IPMI collector are:

- Current power consumption
- Minimum power consumption in the sampling period
- Maximum power consumption in the sampling period
- Average power consumption in the sampling period

Current exporter is capable of auto detecting the IPMI implementation and using
the one that is found.

### Redfish collector

<!-- markdown-link-check-disable -->
The Redfish collector reports the current power usage by the node reported by
[Redfish Chassis Power](https://www.dell.com/support/manuals/fr-fr/idrac7-8-lifecycle-controller-v2.30.30.30/redfish_v2.30.30.30/power?guid=guid-aba3700c-8b2b-4d9b-9c89-73c3177055cc&lang=en-us) specification.
Redfish is a newer server management protocol that succeeds IPMI. If IPMI DCMI
is not available (or the vendor chose to disable it in favor of Redfish),
this collector can be used to fetch the total power consumption of the server.

Redfish reports the power consumption stats for each chassis, and the collector exports
power readings for all the different types of chassis using the `chassis` label. For each
chassis, the metrics exposed by the Redfish collector are:

- Current power consumption
- Minimum power consumption in the sampling period
- Maximum power consumption in the sampling period
- Average power consumption in the sampling period

### HWMon collector

The HWMon collector reports the power and energy consumption of hardware components
when available. Metrics are read from the `/sys/class/hwmon` directory. Each metric has
a chip name to indicate what component is being monitored and a sensor name if there are
multiple sensors monitoring the component. The list of supported metrics presented by the HWMon
collector are:

- Current power consumption
- Minimum power consumption in the sampling period
- Maximum power consumption in the sampling period
- Average power consumption in the sampling period
- Current energy usage

### Cray's PM Counters Collector

Cray's PM counters collector reports the power consumption of CPU, DRAM, and accelerators like GPUs
(when available) using Cray's internal in-band measurements.

The list of metrics exported by Cray's PM counters is:

- Node-level energy, power, and power limit measurements
- CPU and memory energy, power, and power limit measurements
- Accelerators' energy, power, and power limit measurements (when available)

### RAPL collector

The RAPL collector reports the power consumption of CPU and DRAM (when available) using
the Running Average Power Limit (RAPL) framework. The exporter uses powercap to fetch the
energy counters.

The list of metrics exported by the RAPL collector are:

- RAPL package counters
- RAPL DRAM counters (when available)
- RAPL package power limits (when available)

If the CPU architecture supports more RAPL domains other than CPU and DRAM, they will be
exported as well.

### Emissions collector

The Emissions collector exports emission factors from different sources. Depending on the
source, these factors can be static or dynamic, _i.e.,_ varying in time. Currently,
the different sources supported by the exporter are:

- [Electricity Maps](https://app.electricitymaps.com/map) which is capable of providing
real time emission factors for different countries.
- [Watt Time](https://watttime.org/) provides real time emission factors for different regions.
- [RTE eCO2 Mix](https://www.rte-france.com/en/eco2mix/co2-emissions) provides real time
emission factor for **only France**.
- [OWID](https://ourworldindata.org/co2-and-greenhouse-gas-emissions) provides a static
emission factors for different countries based on historical data.
- A world average value that is based on the available data of world countries.

The exporter will provide emission factors of all available countries from different
sources.

### CPU, meminfo, netdev and infiniband collectors

These collectors export node level metrics. CPU collector exports CPU time in different
modes by parsing `/proc/stat` file. Similarly, meminfo collector exports memory usage
statistics by parsing `/proc/meminfo` file. netdev collector exports network metrics
from different network devices. Finally, infiniband collector exports Infiniband metrics
from different IB devices. These collectors are heavily inspired by
[`node_exporter`](https://github.com/prometheus/node_exporter).

These metrics are mainly used to estimate the proportion of CPU and memory usage by individual compute units and to estimate the energy consumption of compute units
based on these proportions.

## Grafana Alloy target discovery

Grafana Alloy provides an [eBPF based continuous profiling](https://grafana.com/docs/alloy/latest/reference/components/pyroscope/pyroscope.ebpf/)
component. It needs a list of targets (processes in the current case) and label those
targets appropriately with a unique identifier of the compute unit. For instance, for a
given compute unit (like batch job for SLURM), there can be multiple processes in the
job and we need to provide a list of all these processes PIDs labeled by the ID of
that compute unit to Grafana Alloy. CEEMS exporter can provide a list of these processes
correctly labeled by the compute unit identifier and eventually these profiles will be
aggregated by compute unit identifier on Pyroscope server.

## Metrics

Please look at [Metrics](./metrics.md) that lists all the metrics exposed by the CEEMS
exporter.
