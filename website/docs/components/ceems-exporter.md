---
sidebar_position: 1
---

# CEEMS Exporter

`ceems_exporter` is the Prometheus exporter that exposes individual compute unit 
metrics, RAPL energy, IPMI power consumption, emission factor and GPU to compute unit
mapping.

Currently, the exporter supports only SLURM resource manager. 
`ceems_exporter` provides following collectors:

- Slurm collector: Exports SLURM job metrics like CPU, memory and GPU indices to job ID maps
- IPMI collector: Exports power usage reported by `ipmi` tools
- RAPL collector: Exports RAPL energy metrics
- Emissions collector: Exports emission factor (g eCO2/kWh)
- CPU collector: Exports CPU time in different modes (at node level)
- Meminfo collector: Exports memory related statistics (at node level)

## Slurm collector

`cgroups` created by SLURM do not have any information on job except for its job ID. 
For the jobs with GPUs, we need to get GPU ordinals of each job during the scrape. 
This collector must export GPU ordinal index to job ID map to Prometheus. The actual 
GPU metrics are exported using [dcgm-exporter](https://github.com/NVIDIA/dcgm-exporter). 
To use `dcgm-exporter`, we need to know which GPU is allocated to which 
job and this info is not available post job. Thus, similar approaches as used to retrieve 
SLURM job properties can be used here as well

Currently the exporter supports few different ways to get these job properties.

- Use prolog and epilog scripts to get the GPU to job ID map. Example prolog script 
is provided in the [repo](./etc/slurm/prolog.d/gpujobmap.sh). Similarly, this approach 
needs `--collector.slurm.gpu.job.map.path=/run/gpujobmap` command line option.

- Reading env vars from `/proc`: If the file created by prolog script cannot be found, 
the exporter defaults to reading the `/proc` file system and attempt to job properties
by reading environment variables of processes. However, this needs privileges which 
can be attributed by assigning `CAP_SYS_PTRACE` and `CAP_DAC_READ_SEARCH` capabilities 
to the `ceems_exporter` process. Assigning capabilities to process is discussed 
in [capabilities section](#linux-capabilities).

- Running exporter as `root`: This will assign all available capabilities for the 
`ceems_exporter` process and thus the necessary job properties and GPU maps will be
read from environment variables in `/proc` file system.

It is recommended to use Prolog and Epilog scripts to get job properties and GPU to job ID maps 
as it does not require any privileges and exporter can run completely in the 
userland. If the admins would not want to have the burden of maintaining prolog and 
epilog scripts, it is better to assign capabilities. These two approaches should be 
always favoured to running the exporter as `root`.

## IPMI collector

There are several IPMI implementation available like FreeIPMI, OpenIPMI, IPMIUtil, 
 _etc._ Current exporter is capable of auto detecting the IPMI implementation and using
the one that is found.

:::important[IMPORTANT]

In addition to IPMI, the exporter can scrape energy readings
from Cray's [capmc](https://cray-hpe.github.io/docs-csm/en-10/operations/power_management/cray_advanced_platform_monitoring_and_control_capmc/) interface.

:::

If the host where exporter is running does not use any of the IPMI implementations, 
it is possible to configure the custom command using CLI flag `--collector.ipmi.dcmi.cmd`. 

:::note[NOTE]

Current auto detection mode is only limited to `ipmi-dcmi` (FreeIPMI), `ipmitool` 
(OpenIPMI), `ipmitutil` (IPMIUtils) and `capmc` (Cray) implementations. These binaries 
must be on `PATH` for the exporter to detect them. If a custom IPMI command is used, 
the command must output the power info in 
[one of these formats](https://github.com/mahendrapaipuri/ceems/blob/c031e0e5b484c30ad8b6e2b68e35874441e9d167/pkg/collector/ipmi.go#L35-L92). 
If that is not the case, operators must write a wrapper around the custom IPMI command 
to output the energy info in one of the supported formats.

:::

The exporter is capable of parsing FreeIPMI, IPMITool and IPMIUtil outputs.
If your IPMI implementation does not return an output in 
[one of these formats](https://github.com/mahendrapaipuri/ceems/blob/c031e0e5b484c30ad8b6e2b68e35874441e9d167/pkg/collector/ipmi.go#L35-L92), 
you can write your own wrapper that parses your IPMI implementation's output and 
returns output in one of above formats. 

Generally `ipmi` related commands are available for only `root`. Admins can add a sudoers 
entry to let the user that runs the `ceems_exporter` to execute only necessary 
command that reports the power usage. For instance, in the case of FreeIPMI 
implementation, that sudoers entry will be

```
ceems ALL = NOPASSWD: /usr/sbin/ipmi-dcmi
```
The exporter will automatically execute the command with `sudo`.

Another supported approach is to run the subprocess `ipmi-dcmi` command as root. In this 
approach, the subprocess will be spawned as root to be able to execute the command. 
This needs `CAP_SETUID` and `CAP_SETGID` capabilities in order to able use `setuid` and
`setgid` syscalls.

## RAPL collector

For the kernels that are `<5.3`, there is no special configuration to be done. If the 
kernel version is `>=5.3`, RAPL metrics are only available for `root`. The capability 
`CAP_DAC_READ_SEARCH` should be able to circumvent this restriction although this has 
not been tested. Another approach is to add a ACL rule on the `/sys/fs/class/powercap` 
directory to give read permissions to the user that is running `ceems_exporter`.

## Emissions collector

The only CLI flag to configure for emissions collector is 
`--collector.emissions.country.code` and set it to 
[ISO 2 Country Code](https://en.wikipedia.org/wiki/ISO_3166-1_alpha-2). By setting 
an environment variable `EMAPS_API_TOKEN`, emission factors from 
[Electricity Maps](https://app.electricitymaps.com/map) data will also be reported.

If country is set to France, emission factor data from 
[RTE eCO2 Mix](https://www.rte-france.com/en/eco2mix/co2-emissions) will also be reported. 
There is no need to pass any API token.

## CPU and meminfo collectors

Both collectors export node level metrics. CPU collector export CPU time in different
modes by parsing `/proc/stat` file. Similarly, meminfo collector exports memory usage 
statistics by parsing `/proc/meminfo` file. These collectors are heavily inspired from 
[`node_exporter`](https://github.com/prometheus/node_exporter). 

These metrics are mainly used to estimate the proportion of CPU and memory usage by the 
individual compute units and to estimate the energy consumption of compute unit 
based on these proportions.
