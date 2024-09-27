---
sidebar_position: 6
---

# Security

## Privileges

CEEMS exporter needs access to IPMI interface to get current power consumption which is
only available for privileged users and/or processes. Thus, a trivial
solution is to run CEEMS exporter as `root`. For accessing metrics from `perf` subsystem
or to use eBPF, CEEMS exporter needs different types of privileges. Similarly, in order to which GPU has been
assigned to which compute unit, we need to either introspect compute unit's
environment variables (for SLURM resource manager), `libvirt` XML file for VM (for Openstack),
_etc_. These actions need privileges as well. However, these privileges are only
needed at specific points of execution in the exporter code and hence, running the exporter
as `root` is an overkill.

We can leverage
[Linux Capabilities](https://man7.org/linux/man-pages/man7/capabilities.7.html) to
assign just the necessary privileges to the process. Moreover, CEEMS apps are
capability aware, _i.e.,_ the app will assume the privileges only when they are
required during the code execution. It executes the piece of code that needs
privileges in a separate thread and destroys the thread after finishing the
execution to ensure no other goroutine will be scheduled on that thread. This ensures
the whole exporter code runs as unprivileged process most of the time and raises the
privileges only when necessary. This strategy makes CEEMS apps
[capability aware](https://tbhaxor.com/understanding-linux-capabilities/).

:::important[IMPORTANT]

Even if operator starts any of the CEEMS app as `root` user, the app will just
keep the capabilities that it needs and drops rest of them and switch to
`nobody` user to ensure it runs with minimum necessary privileges. As stated
above, it only raises the capabilities when needed and run rest of the time
as unprivileged process.

:::

## Linux capabilities

### CEEMS Exporter

For different collectors of CEEMS exporter, different capabilities are needed. The
following list summaries the capabilities needed for each collector:

- `ipmi_dcmi`: `cap_setuid` and `cap_setgid` to execute IPMI command as `root`.
- `slurm`: `cap_sys_ptrace` and `cap_dac_read_search` to be able to access processes'
environment variables to get GPU indices of a given compute job. If `--collector.slurm.gpu-job-map-path`
is used, these capabilities wont be needed.
- `perf`: `cap_perfmon` to be able to open perf events. `cap_sys_ptrace` and `cap_dac_read_search`
to be able to access processes' environment variables when `--collector.slurm.perf-env-var` is
used.
- `ebpf`: `cap_bpf` and `cap_perfmon` to be able to load and access BPP programs and maps. For
kernels < 5.11, `cap_sys_resource` is necessary to increase locked memory for loading BPF
programs.
- `rapl`: `cap_dac_read_search` when kernels > 5.3 is used as RAPL counters from this kernel
version is only access to `root`.

### CEEMS API Server

Currently, SLURM resource manager of CEEMS API server only supports fetching job data from
`sacct` command. It is possible to fetch jobs of all users for only `slurm` system user
or `root` user. Thus, `cap_setuid` and `cap_setgid` are needed to execute this command
as either `slurm` or `root` user.

If operators would like to add the user under which CEEMS API server is running under to
SLURM users list, these capabilities wont be needed anymore.

### CEEMS LB

CEEMS LB do not need any special privileges and capabilities.

## Systemd

If CEEMS components are installed using [RPM/DEB packages](../installation/os-packages.md), a basic
systemd unit file will be installed to start the service. However, when they are
installed manually using [pre-compiled binaries](../installation/pre-compiled-binaries.md), it is
necessary to install and configure `systemd` unit files to manage the service.

Linux capabilities can be assigned to either file or process. For instance, capabilities
on the `ceems_exporter` and `ceems_api_server` binaries can be set as follows:

```bash
sudo setcap cap_sys_ptrace,cap_dac_read_search,cap_setuid,cap_setgid+p /full/path/to/ceems_exporter
sudo setcap cap_setuid,cap_setgid+p /full/path/to/ceems_api_server
```

This will assign all the capabilities that are necessary to run `ceems_exporter`
for all the collectors. Using file based capabilities will
expose those capabilities to anyone on the system that have execute permissions on the
binary. Although, it does not pose a big security concern, it is better to assign
capabilities to a process.

As operators tend to run the exporter within a `systemd` unit file, we can assign
capabilities to the process rather than file using `AmbientCapabilities`
directive of the `systemd`. An example is as follows:

```ini
[Service]
ExecStart=/usr/local/bin/ceems_exporter --collector.slurm --collector.perf.hardware-events --collector.ebpf.io-metrics
AmbientCapabilities=CAP_SYS_PTRACE CAP_DAC_READ_SEARCH CAP_SETUID CAP_SETGID CAP_PERFMON CAP_BPF
```

Note that it is bare minimum service file and it is only to demonstrate on how to use
`AmbientCapabilities`. Production ready [service files examples]((https://github.com/mahendrapaipuri/ceems/tree/main/build/package))
are provided in repo.

## Containers

If operators would like to deploy exporter inside a container, it is necessary to give
full privileges to the container for the exporter to access different files under
`/dev`, `/sys` and `/proc`. As stated in [capabilities](#linux-capabilities) section,
exporter will drop all the capabilities and keep only necessary ones based on
runtime config and hence, running container as privileged does not pose a major
security risk.
