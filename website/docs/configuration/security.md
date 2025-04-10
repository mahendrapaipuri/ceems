---
sidebar_position: 9
---

# Security

## Privileges

The CEEMS exporter needs access to the IPMI interface to get current power consumption, which is only available to privileged users and/or processes. Thus, a trivial solution is to run the CEEMS exporter as `root`. For accessing metrics from the `perf` subsystem or using eBPF, the CEEMS exporter needs different types of privileges. Similarly, to determine which GPU has been assigned to which compute unit, we need to either introspect the compute unit's environment variables (for the SLURM resource manager) or the `libvirt` XML file for VMs (for OpenStack), etc. These actions also require privileges. However, these privileges are only needed at specific points of execution in the exporter code, so running the exporter as `root` is overkill.

We can leverage [Linux Capabilities](https://man7.org/linux/man-pages/man7/capabilities.7.html) to assign just the necessary privileges to the process. Moreover, CEEMS apps are capability-aware, meaning the app will assume privileges only when they are required during code execution. It executes the piece of code that needs privileges in a separate thread and destroys the thread after finishing the execution to ensure no other goroutine will be scheduled on that thread. This ensures the whole exporter code runs as an unprivileged process most of the time and raises privileges only when necessary. This strategy makes CEEMS apps [capability aware](https://tbhaxor.com/understanding-linux-capabilities/).

:::important[IMPORTANT]

Even if the operator starts any of the CEEMS apps as the `root` user, the app will keep only the capabilities it needs, drop the rest, and switch to the `nobody` user to ensure it runs with the minimum necessary privileges. As stated above, it only raises the capabilities when needed and runs the rest of the time as an unprivileged process.

:::

## Linux capabilities

### CEEMS Exporter

Different collectors of the CEEMS exporter require different capabilities. The following list summarizes the capabilities needed for each collector:

- `ipmi_dcmi`: `cap_setuid` and `cap_setgid` to execute IPMI commands as `root` when third-party libraries are used. `cap_dac_override` when the pure Golang implementation is used to communicate with the device `/dev/ipmi0`.
- `redfish`: `cap_dac_override` to discover the BMC IP address when it is not provided via the configuration file.
- `slurm`: `cap_sys_ptrace` and `cap_dac_read_search` to access processes' environment variables to get GPU indices of a given compute job. If `--collector.slurm.gpu-job-map-path` is used, these capabilities won't be needed.
- `libvirt`: `cap_dac_read_search` to read the instance's XML file.
- `perf`: `cap_perfmon` to open perf events. `cap_sys_ptrace` and `cap_dac_read_search` to access processes' environment variables when `--collector.slurm.perf-env-var` is used.
- `ebpf`: `cap_bpf` and `cap_perfmon` to load and access BPF programs and maps. For kernels < 5.11, `cap_sys_resource` is necessary to increase locked memory for loading BPF programs.
- `rdma`: `cap_setuid` and `cap_setgid` to enable Per-PID counters for RDMA QPs.
- `rapl`: `cap_dac_read_search` when kernels > 5.3 are used, as RAPL counters from this kernel version are only accessible to `root`.

### CEEMS API Server

Currently, the SLURM resource manager of the CEEMS API server only supports fetching job data from the `sacct` command. It is possible to fetch jobs of all users only for the `slurm` system user or `root` user. Thus, `cap_setuid` and `cap_setgid` are needed to execute this command as either the `slurm` or `root` user.

If operators would like to add the user under which the CEEMS API server is running to the SLURM users list, these capabilities won't be needed anymore.

### CEEMS LB

The CEEMS LB does not need any special privileges or capabilities.

## Systemd

If CEEMS components are installed using [RPM/DEB packages](../installation/os-packages.md), a basic systemd unit file will be installed to start the service. However, when they are installed manually using [pre-compiled binaries](../installation/pre-compiled-binaries.md), it is necessary to install and configure `systemd` unit files to manage the service.

Linux capabilities can be assigned to either a file or a process. For instance, capabilities on the `ceems_exporter` and `ceems_api_server` binaries can be set as follows:

```bash
sudo setcap cap_sys_ptrace,cap_dac_read_search,cap_setuid,cap_setgid+p /full/path/to/ceems_exporter
sudo setcap cap_setuid,cap_setgid+p /full/path/to/ceems_api_server
```

This will assign all the capabilities necessary to run `ceems_exporter` for all collectors. Using file-based capabilities will expose those capabilities to anyone on the system who has execute permissions on the binary. Although this does not pose a major security concern, it is advisable to assign capabilities to a process.

As operators tend to run the exporter within a `systemd` unit file, we can assign capabilities to the process rather than the file using the `AmbientCapabilities` directive of `systemd`. An example is as follows:

```ini
[Service]
ExecStart=/usr/local/bin/ceems_exporter --collector.slurm --collector.perf.hardware-events --collector.ebpf.io-metrics --collector.ipmi_dcmi --collector.ipmi_dcmi.force-native-mode
AmbientCapabilities=CAP_SYS_PTRACE CAP_DAC_READ_SEARCH CAP_DAC_OVERRIDE CAP_PERFMON CAP_BPF CAP_SYS_RESOURCE
```

Note that this is a bare minimum service file and is only meant to demonstrate how to use `AmbientCapabilities`. Production-ready [service file examples](https://github.com/mahendrapaipuri/ceems/tree/main/build/package) are provided in the repository.

## Containers

If operators would like to deploy the exporter inside a container, it is necessary to give full privileges to the container for the exporter to access different files under `/dev`, `/sys`, and `/proc`. As stated in the [capabilities](#linux-capabilities) section, the exporter will drop all capabilities and keep only the necessary ones based on runtime configuration; hence, running the container as privileged does not pose a major security risk.
