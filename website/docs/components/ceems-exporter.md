---
sidebar_position: 1
---

# CEEMS Exporter

## Background

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

Slurm collector exports the job related metrics like usage of CPU, DRAM, RDMA, _etc_. 
This is done by walking through the cgroups created by SLURM daemon on compute node on 
every scrape request. As walking through the cgroups pseudo file system is _very cheap_, 
this will zero zero to negligible impact on the actual job.

The exporter has been heavily inspired by 
[cgroups_exporter](https://github.com/treydock/cgroup_exporter) and it supports both 
cgroups **v1** and **v2**. For jobs with GPUs, we must the GPU ordinals allocated to 
each job so that we can match GPU metrics scrapped by either 
[dcgm-exporter](https://github.com/NVIDIA/dcgm-exporter) or 
[amd-smi-exporter](https://github.com/amd/amd_smi_exporter) to jobs. Unfortunately, 
this information is not available post-mortem of the job and hence, we need to export 
the mapping related to job ID to GPU ordinals. 

:::warning[WARNING]

For SLURM collector to work properly, SLURM needs to be configured well to enable all 
the available cgroups controllers. At least `cpu` and `memory` controllers must be 
enabled if not cgroups will not contain any accounting information. Without `cpu` 
and `memory` accounting information, it is not possible to estimate energy consumption 
of the job.

:::

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

## IPMI collector

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

## RAPL collector

RAPL collector reports the power consumption of CPU and DRAM (when available) using 
Running Average Power Limit (RAPL) framework. The exporter uses powercap to fetch the 
energy counters. 

List of metrics exported by RAPL collector are:

- RAPL package counters
- RAPL DRAM counters (when available)

If the CPU architecture supports more RAPL domains otherthan CPU and DRAM, they will be 
exported as well.

## Emissions collector

Emissions collector exports emissions factors from different sources. Depending on the 
source, these factors can be static or dynamic, _i.e.,_ varying in time. Currently, 
different sources supported by the exporter are:

- [Electricity Maps](https://app.electricitymaps.com/map) which is capable of providing 
real time emission factors for different countries.
- [RTE eCO2 Mix](https://www.rte-france.com/en/eco2mix/co2-emissions) provides real time
emission factor for **only France**.
- [OWID](https://ourworldindata.org/co2-and-greenhouse-gas-emissions) provides a static 
emission factors for different countries based on historical data.
- A world average value that is based on the data of available data of the world countries.

The exporter will export the emission factors of all available countries from different
sources.

## CPU and meminfo collectors

Both collectors export node level metrics. CPU collector export CPU time in different
modes by parsing `/proc/stat` file. Similarly, meminfo collector exports memory usage 
statistics by parsing `/proc/meminfo` file. These collectors are heavily inspired from 
[`node_exporter`](https://github.com/prometheus/node_exporter). 

These metrics are mainly used to estimate the proportion of CPU and memory usage by the 
individual compute units and to estimate the energy consumption of compute unit 
based on these proportions.
