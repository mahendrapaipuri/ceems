# Compute Energy & Emissions Monitoring Stack (CEEMS)

|         |                                                                                                                                                                                                                                                                                                                                                                                                                 |
| ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| CI/CD   | [![ci](https://github.com/mahendrapaipuri/ceems/workflows/CI/badge.svg)](https://github.com/mahendrapaipuri/ceems) [![CircleCI](https://dl.circleci.com/status-badge/img/circleci/8jSYT1wyKY8mKQRTqNLThX/TzM1Mr3AEAqmehnoCde19R/tree/main.svg?style=svg&circle-token=28db7268f3492790127da28e62e76b0991d59c8b)](https://dl.circleci.com/status-badge/redirect/circleci/8jSYT1wyKY8mKQRTqNLThX/TzM1Mr3AEAqmehnoCde19R/tree/main)  [![Coverage](https://img.shields.io/badge/Coverage-63.6%25-yellow)](https://github.com/mahendrapaipuri/ceems/actions/workflows/ci.yml?query=branch%3Amain)                                                                                          |
| Docs    | [![docs](https://img.shields.io/badge/docs-passing-green?style=flat&link=https://github.com/mahendrapaipuri/ceems/blob/main/README.md)](https://github.com/mahendrapaipuri/ceems/blob/main/README.md)                                                                                                                                                                                                                               |
| Package | [![Release](https://img.shields.io/github/v/release/mahendrapaipuri/ceems.svg?include_prereleases)](https://github.com/mahendrapaipuri/ceems/releases/latest)                                                                                                                                                                     |
| Meta    | [![GitHub License](https://img.shields.io/github/license/mahendrapaipuri/ceems)](https://gitlab.com/mahendrapaipuri/ceems) [![Go Report Card](https://goreportcard.com/badge/github.com/mahendrapaipuri/ceems)](https://goreportcard.com/report/github.com/mahendrapaipuri/ceems) [![code style](https://img.shields.io/badge/code%20style-gofmt-blue.svg)](https://pkg.go.dev/cmd/gofmt) |


Compute Energy & Emissions Monitoring Stack (CEEMS) contains a Prometheus exporter to 
export metrics of compute instance units and a REST API server which is meant to be used
as JSON datasource in Grafana that exposes the metadata and aggregated metrics of each 
compute unit. Optionally, it includes a TSDB load balancer that supports basic load 
balancing functionality based on retention periods of two or more TSDBs. 

"Compute Unit" in the current context has a wider scope. It can be a batch job in HPC, 
a VM in cloud, a pod in k8s, _etc_. The main objective of the repository is to quantify 
the energy consumed and estimate emissions by each "compute unit". The repository itself 
does not provide any frontend apps to show dashboards and it is meant to use along 
with Grafana and Prometheus to show statistics to users. 

## Design objectives

### CPU, memory and IO metrics

The idea we are leveraging here is that every resource manager has to resort to cgroups 
on Linux to manage CPU, memory and IO resources. Each resource manager does it 
differently but the take away here is that the accounting information is readily 
available in the cgroups. By walking through the cgroups file system, we can gather the 
metrics that map them to a particular compute unit as resource manager tends to create 
cgroups for each compute unit with some sort of identifier attached to it.

This is a distributed approach where exporter will run on each compute node. Whenever 
Prometheus make a scrape request, the exporter will walk through cgroup file system and 
exposes the data to Prometheus. As reading cgroups file system is relatively cheap, 
there is a very little overhead running this daemon service. On average the exporter 
takes less than 20 MB of memory. 

### Energy consumption

In an age where green computing is becoming more and more important, it is essential to
expose the energy consumed by the compute units to the users to make them more aware. 
Most of energy measurement tools are based on 
[RAPL](https://www.kernel.org/doc/html/next/power/powercap/powercap.html) which reports 
energy consumption from CPU and memory. It does not report consumption from other 
peripherals like PCIe, network, disk, _etc_. 

To address this, the current exporter will expose IPMI power statistics in addition to 
RAPL metrics. IPMI measurements are generally made at the node level which includes 
consumption by _most_ of the components. However, the implementations are vendor 
dependent and it is desirable to validate with them before reading too much into the 
numbers. In any case, this is the only complete metric we can get our hands on without 
needing to install any additional hardware like Wattmeters. 

This node level power consumption can be split into consumption of individual compute units
by using relative CPU times used by the compute unit. Although, this is not an exact 
estimation of power consumed by the compute unit, it stays a very good approximation.

### Emissions

The exporter is capable of exporting emission factors from different data sources 
which can be used to estimate equivalent CO2 emissions. Currently, for 
France, a _real_ time emission factor will be used that is based on 
[RTE eCO2 mix data](https://www.rte-france.com/en/eco2mix/co2-emissions). Besides, 
retrieving emission factors from [Electricity Maps](https://app.electricitymaps.com/map) 
is also supported provided that API token is provided. Electricity Maps provide 
emission factor data for most of the countries. A static emission factor from historic 
data is also provided from [OWID data](https://github.com/owid/co2-data). Finally, a 
constant global average emission factor is also exported.

Emissions collector is capable of exporting emission factors from different sources 
based on current environment. We should be able to use appropriate one in Grafana 
dashboards to estimate equivalent CO2 emissions.

### GPU metrics

Currently, only nVIDIA and AMD GPUs are supported. This exporter leverages 
[DCGM exporter](https://github.com/NVIDIA/dcgm-exporter/tree/main) for nVIDIA GPUs and
[AMD SMI exporter](https://github.com/amd/amd_smi_exporter) for AMD GPUs to get GPU metrics of
each compute unit. DCGM/AMD SMI exporters exposes the GPU metrics of each GPU and the 
current exporter only exposes the GPU index to compute unit mapping. These two metrics 
can be used together using PromQL to show the metrics of GPU metrics of a given compute 
unit.

## Current stack objective

Using this stack with Prometheus and Grafana will enable users to have access to time 
series metrics of their compute units be it a batch job, a VM or a pod. The users will 
also able to have information on total energy consumed and total emissions generated 
by their individual workloads, by their project/namespace. 

On the otherhand system admins will be able to list the consumption of energy, emissions, 
CPU time, memory, _etc_ for each projects/namespaces/users. This can be used to generate
reports regularly on the energy usage of the data center.

## Repository contents

This monorepo contains following apps that can be used with Grafana and Prometheus

- `ceems_exporter`: This is the Prometheus exporter that exposes individual compute unit 
metrics, RAPL energy, IPMI power consumption, emission factor and GPU to compute unit
mapping.

- `ceems_api_server`: This is a simple REST API server that exposes projects and compute units 
information of users by querying a SQLite3 DB. 
This server can be used as 
[JSON API DataSource](https://grafana.github.io/grafana-json-datasource/installation/) or 
[Infinity DataSource](https://grafana.com/grafana/plugins/yesoreyeram-infinity-datasource/)
in Grafana to construct dashboards for users. The DB contain aggregate metrics of each 
compute unit along with aggregate metrics of each project.

- `ceems_lb`: This is a basic load balancer meant to work with TSDB instances.

Currently, only SLURM is supported as a resource manager. In future support for Openstack 
and Kubernetes will be added. 

## Getting started

### Install

Pre-compiled binaries, RPMs and deb packages of the apps can be downloaded from the 
[releases](https://github.com/mahendrapaipuri/ceems/releases/).

### Build

As the `ceems_api_server` uses SQLite3 as DB backend, we are dependent on CGO for 
compiling that app. On the other hand, `ceems_exporter` is a pure GO application. 
Thus, in order to build from sources, users need to execute two build commands

```
make build
```

that builds `ceems_exporter` binary and

```
CGO_BUILD=1 make build
```

which builds `ceems_api_server` and `ceems_lb` apps.

All the applications will be placed in `bin` folder in the root of the repository

### Running tests

In the same way, to run unit and end-to-end tests for apps, it is enough to run

```
make tests
CGO_BUILD=1 make tests
```

## CEEMS Exporter

Currently, the exporter supports only SLURM resource manager. 
`ceems_exporter` provides following collectors:

- Slurm collector: Exports SLURM job metrics like CPU, memory and GPU indices to job ID maps
- IPMI collector: Exports power usage reported by `ipmi` tools
- RAPL collector: Exports RAPL energy metrics
- Emissions collector: Exports emission factor (g eCO2/kWh)
- CPU collector: Exports CPU time in different modes (at node level)
- Meminfo collector: Exports memory related statistics (at node level)

### Slurm collector

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

### IPMI collector

There are several IPMI implementation available like FreeIPMI, OpenIPMI, IPMIUtil, 
 _etc._ Current exporter is capable of auto detecting the IPMI implementation and using
the one that is found.

> [!IMPORTANT]
> In addition to IPMI, the exporter can scrape energy readings
from Cray's [capmc](https://cray-hpe.github.io/docs-csm/en-10/operations/power_management/cray_advanced_platform_monitoring_and_control_capmc/) interface.

If the host where exporter is running does not use any of the IPMI implementations, 
it is possible to configure the custom command using CLI flag `--collector.ipmi.dcmi.cmd`. 

> [!NOTE]
> Current auto detection mode is only limited to `ipmi-dcmi` (FreeIPMI), `ipmitool` 
(OpenIPMI), `ipmitutil` (IPMIUtils) and `capmc` (Cray) implementations. These binaries 
must be on `PATH` for the exporter to detect them. If a custom IPMI command is used, 
the command must output the power info in 
[one of these formats](https://github.com/mahendrapaipuri/ceems/blob/c031e0e5b484c30ad8b6e2b68e35874441e9d167/pkg/collector/ipmi.go#L35-L92). 
If that is not the case, operators must write a wrapper around the custom IPMI command 
to output the energy info in one of the supported formats.

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

### RAPL collector

For the kernels that are `<5.3`, there is no special configuration to be done. If the 
kernel version is `>=5.3`, RAPL metrics are only available for `root`. The capability 
`CAP_DAC_READ_SEARCH` should be able to circumvent this restriction although this has 
not been tested. Another approach is to add a ACL rule on the `/sys/fs/class/powercap` 
directory to give read permissions to the user that is running `ceems_exporter`.

### Emissions collector

The only CLI flag to configure for emissions collector is 
`--collector.emissions.country.code` and set it to 
[ISO 2 Country Code](https://en.wikipedia.org/wiki/ISO_3166-1_alpha-2). By setting 
an environment variable `EMAPS_API_TOKEN`, emission factors from 
[Electricity Maps](https://app.electricitymaps.com/map) data will also be reported.

If country is set to France, emission factor data from 
[RTE eCO2 Mix](https://www.rte-france.com/en/eco2mix/co2-emissions) will also be reported. 
There is no need to pass any API token.

### CPU and meminfo collectors

Both collectors export node level metrics. CPU collector export CPU time in different
modes by parsing `/proc/stat` file. Similarly, meminfo collector exports memory usage 
statistics by parsing `/proc/meminfo` file. These collectors are heavily inspired from 
[`node_exporter`](https://github.com/prometheus/node_exporter). 

These metrics are mainly used to estimate the proportion of CPU and memory usage by the 
individual compute units and to estimate the energy consumption of compute unit 
based on these proportions.

## CEEMS API server

As discussed in the introduction, `ceems_api_server` 
exposes usage and compute unit details of users _via_ API end points. This data will be 
gathered from the underlying resource manager at a configured interval of time and 
keep it in a local DB.

## CEEMS Load Balancer

Taking Prometheus TSDB as an example, Prometheus advises to use local file system to store 
the data. This ensure performance and data integrity. However, storing data on local 
disk is not fault tolerant unless data is replicated elsewhere. There are cloud native 
projects like [Thanos](https://thanos.io/), [Cortex](https://cortexmetrics.io/) to 
address this issue. This load balancer is meant 
to provide the basic functionality proposed by Thanos, Cortex, _etc_.

The core idea is to replicate the Prometheus data using 
[Prometheus' remote write](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#remote_write) 
functionality onto a remote storage which 
is fault tolerant and have higher storage capacity but with a degraded query performance. 
In this scenario, we have two TSDBs with following characteristics:

- TSDB using local disk: faster query performance with limited storage space
- TSDB using remote storage: slower query performance with bigger storage space

TSDB using local disk ("hot" instance) will have shorter retention period and the 
one using remote storage ("cold" instance)
can have longer retention. CEEMS load balancer is capable of introspecting the query and
then routing the request to either "hot" or "cold" instances of TSDB.

Besides CEEMS load balancer is capable of providing basic access control policies of 
TSDB if the DB of CEEMS API server is provided. It means when a user makes a TSDB query 
for a given compute unit identified by a `uuid`, CEEMS load balancer will check if the 
user owns that compute unit by check with the DB and decide to proxy the request to 
TSDB or not. This is very handy as Grafana does not impose any access control on datasources
and current load balancer can provide such functionality.

<!-- In the case of SLURM, the app executes `sacct` command to get 
info on jobs. However, `sacct` command needs to be executed as either `root` or `slurm` 
user to get job details of _all_ users. 

Current implementation does following during DB initialization

- If current user is `root` or `slurm`, no new privileges are needed. As these two users
are capable of pulling job data of all users. 

- Spawn a subprocess for `sacct` and execute it as `slurm` user. If the subprocess 
execution succeeds, it will be used in periodic update of DB. For this approach to work,
`CAP_SETUID` and `CAP_SETGID` capabilities must be assigned to current process.

- If the above approach fails, we attempt to run `sacct` with `sudo`. This required 
that we need to give the user permission to use `sudo` by adding an entry into 
sudoers file. If it succeeds, this method will be used in periodic update.

If none of the above approaches work, `sacct` command will be executed natively, _i.e.,_
we will run the command with whatever option is passed to `--slurm.sacct.path`. This 
would work if admins use their own wrappers to `sacct` that does privilege escalation 
using different methods like `setuid` sticky bit. -->

## Linux capabilities

Linux capabilities can be assigned to either file or process. For instance, capabilities 
on the `ceems_exporter` and `ceems_api_server` binaries can be set as follows:

```
sudo setcap cap_sys_ptrace,cap_dac_read_search,cap_setuid,cap_setgid+ep /full/path/to/ceems_exporter
sudo setcap cap_setuid,cap_setgid+ep /full/path/to/ceems_api_server
```

This will assign all the capabilities that are necessary to run `ceems_exporter` 
for all the collectors stated in the above section. Using file based capabilities will 
expose those capabilities to anyone on the system that have execute permissions on the 
binary. Although, it does not pose a big security concern, it is better to assign 
capabilities to a process. 

As admins tend to run the exporter within a `systemd` unit file, we can assign 
capabilities to the process rather than file using `AmbientCapabilities` 
directive of the `systemd`. An example is as follows:

```
[Service]
ExecStart=/usr/local/bin/ceems_exporter
AmbientCapabilities=CAP_SYS_PTRACE CAP_DAC_READ_SEARCH CAP_SETUID CAP_SETGID
```

Note that it is bare minimum service file and it is only to demonstrate on how to use 
`AmbientCapabilities`. Production ready service files examples are provided in 
[repo](./init/systemd)

## Usage

### `ceems_exporter`

Using prolog and epilog scripts approach and `sudo` for `ipmi`, 
`ceems_exporter` can be started as follows

```
/path/to/ceems_exporter \
    --collector.slurm.job.props.path="/run/slurmjobprops" \
    --collector.slurm.gpu.type="nvidia" \
    --collector.slurm.gpu.job.map.path="/run/gpujobmap" \
    --log.level="debug"
```

This will start exporter server on default 9010 port. Metrics can be consulted using 
`curl http://localhost:9010/metrics` command which will give an output as follows:

```
# HELP ceems_compute_unit_cpu_psi_seconds Total CPU PSI in seconds
# TYPE ceems_compute_unit_cpu_psi_seconds gauge
ceems_compute_unit_cpu_psi_seconds{hostname="",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 0
ceems_compute_unit_cpu_psi_seconds{hostname="",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 0
ceems_compute_unit_cpu_psi_seconds{hostname="",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 0
# HELP ceems_compute_unit_cpu_system_seconds_total Total job CPU system seconds
# TYPE ceems_compute_unit_cpu_system_seconds_total counter
ceems_compute_unit_cpu_system_seconds_total{hostname="",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 115.777502
ceems_compute_unit_cpu_system_seconds_total{hostname="",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 115.777502
ceems_compute_unit_cpu_system_seconds_total{hostname="",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 115.777502
# HELP ceems_compute_unit_cpu_user_seconds_total Total job CPU user seconds
# TYPE ceems_compute_unit_cpu_user_seconds_total counter
ceems_compute_unit_cpu_user_seconds_total{hostname="",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 60375.292848
ceems_compute_unit_cpu_user_seconds_total{hostname="",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 60375.292848
ceems_compute_unit_cpu_user_seconds_total{hostname="",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 60375.292848
# HELP ceems_compute_unit_cpus Total number of job CPUs
# TYPE ceems_compute_unit_cpus gauge
ceems_compute_unit_cpus{hostname="",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 2
ceems_compute_unit_cpus{hostname="",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 2
ceems_compute_unit_cpus{hostname="",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 2
# HELP ceems_compute_unit_gpu_index_flag Indicates running job on GPU, 1=job running
# TYPE ceems_compute_unit_gpu_index_flag gauge
ceems_compute_unit_gpu_index_flag{account="testacc",gpuuuid="20170005280c",hindex="-gpu-3",hostname="",index="3",manager="slurm",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 1
ceems_compute_unit_gpu_index_flag{account="testacc",gpuuuid="20180003050c",hindex="-gpu-2",hostname="",index="2",manager="slurm",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 1
ceems_compute_unit_gpu_index_flag{account="testacc2",gpuuuid="20170000800c",hindex="-gpu-0",hostname="",index="0",manager="slurm",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 1
ceems_compute_unit_gpu_index_flag{account="testacc3",gpuuuid="20170003580c",hindex="-gpu-1",hostname="",index="1",manager="slurm",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 1
# HELP ceems_compute_unit_memory_cache_bytes Memory cache used in bytes
# TYPE ceems_compute_unit_memory_cache_bytes gauge
ceems_compute_unit_memory_cache_bytes{hostname="",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 0
ceems_compute_unit_memory_cache_bytes{hostname="",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 0
ceems_compute_unit_memory_cache_bytes{hostname="",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 0
# HELP ceems_compute_unit_memory_fail_count Memory fail count
# TYPE ceems_compute_unit_memory_fail_count gauge
ceems_compute_unit_memory_fail_count{hostname="",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 0
ceems_compute_unit_memory_fail_count{hostname="",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 0
ceems_compute_unit_memory_fail_count{hostname="",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 0
# HELP ceems_compute_unit_memory_psi_seconds Total memory PSI in seconds
# TYPE ceems_compute_unit_memory_psi_seconds gauge
ceems_compute_unit_memory_psi_seconds{hostname="",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 0
ceems_compute_unit_memory_psi_seconds{hostname="",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 0
ceems_compute_unit_memory_psi_seconds{hostname="",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 0
# HELP ceems_compute_unit_memory_rss_bytes Memory RSS used in bytes
# TYPE ceems_compute_unit_memory_rss_bytes gauge
ceems_compute_unit_memory_rss_bytes{hostname="",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 4.098592768e+09
ceems_compute_unit_memory_rss_bytes{hostname="",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 4.098592768e+09
ceems_compute_unit_memory_rss_bytes{hostname="",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 4.098592768e+09
# HELP ceems_compute_unit_memory_total_bytes Memory total in bytes
# TYPE ceems_compute_unit_memory_total_bytes gauge
ceems_compute_unit_memory_total_bytes{hostname="",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 4.294967296e+09
ceems_compute_unit_memory_total_bytes{hostname="",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 4.294967296e+09
ceems_compute_unit_memory_total_bytes{hostname="",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 4.294967296e+09
# HELP ceems_compute_unit_memory_used_bytes Memory used in bytes
# TYPE ceems_compute_unit_memory_used_bytes gauge
ceems_compute_unit_memory_used_bytes{hostname="",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 4.111491072e+09
ceems_compute_unit_memory_used_bytes{hostname="",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 4.111491072e+09
ceems_compute_unit_memory_used_bytes{hostname="",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 4.111491072e+09
# HELP ceems_compute_unit_memsw_fail_count Swap fail count
# TYPE ceems_compute_unit_memsw_fail_count gauge
ceems_compute_unit_memsw_fail_count{hostname="",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 0
ceems_compute_unit_memsw_fail_count{hostname="",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 0
ceems_compute_unit_memsw_fail_count{hostname="",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 0
# HELP ceems_compute_unit_memsw_total_bytes Swap total in bytes
# TYPE ceems_compute_unit_memsw_total_bytes gauge
ceems_compute_unit_memsw_total_bytes{hostname="",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 1.6042172416e+10
ceems_compute_unit_memsw_total_bytes{hostname="",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 1.6042172416e+10
ceems_compute_unit_memsw_total_bytes{hostname="",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 1.6042172416e+10
# HELP ceems_compute_unit_memsw_used_bytes Swap used in bytes
# TYPE ceems_compute_unit_memsw_used_bytes gauge
ceems_compute_unit_memsw_used_bytes{hostname="",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 0
ceems_compute_unit_memsw_used_bytes{hostname="",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 0
ceems_compute_unit_memsw_used_bytes{hostname="",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 0
# HELP ceems_compute_unit_rdma_hca_handles Current number of RDMA HCA handles
# TYPE ceems_compute_unit_rdma_hca_handles gauge
ceems_compute_unit_rdma_hca_handles{device="hfi1_0",hostname="",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 479
ceems_compute_unit_rdma_hca_handles{device="hfi1_0",hostname="",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 289
ceems_compute_unit_rdma_hca_handles{device="hfi1_1",hostname="",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 1479
ceems_compute_unit_rdma_hca_handles{device="hfi1_2",hostname="",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 2479
# HELP ceems_compute_unit_rdma_hca_objects Current number of RDMA HCA objects
# TYPE ceems_compute_unit_rdma_hca_objects gauge
ceems_compute_unit_rdma_hca_objects{device="hfi1_0",hostname="",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 479
ceems_compute_unit_rdma_hca_objects{device="hfi1_0",hostname="",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 289
ceems_compute_unit_rdma_hca_objects{device="hfi1_1",hostname="",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 1479
ceems_compute_unit_rdma_hca_objects{device="hfi1_2",hostname="",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 2479
# HELP ceems_compute_units Total number of jobs
# TYPE ceems_compute_units gauge
ceems_compute_units{hostname="",manager="slurm"} 3
# HELP ceems_cpu_count Number of CPUs.
# TYPE ceems_cpu_count gauge
ceems_cpu_count{hostname=""} 8
# HELP ceems_cpu_seconds_total Seconds the CPUs spent in each mode.
# TYPE ceems_cpu_seconds_total counter
ceems_cpu_seconds_total{hostname="",mode="idle"} 89790.04
ceems_cpu_seconds_total{hostname="",mode="iowait"} 35.52
ceems_cpu_seconds_total{hostname="",mode="irq"} 0.02
ceems_cpu_seconds_total{hostname="",mode="nice"} 6.12
ceems_cpu_seconds_total{hostname="",mode="softirq"} 39.44
ceems_cpu_seconds_total{hostname="",mode="steal"} 0
ceems_cpu_seconds_total{hostname="",mode="system"} 1119.22
ceems_cpu_seconds_total{hostname="",mode="user"} 3018.54
# HELP ceems_exporter_build_info A metric with a constant '1' value labeled by version, revision, branch, goversion from which ceems_exporter was built, and the goos and goarch for the build.
# TYPE ceems_exporter_build_info gauge
# HELP ceems_ipmi_dcmi_avg_watts Average Power consumption in watts
# TYPE ceems_ipmi_dcmi_avg_watts gauge
ceems_ipmi_dcmi_avg_watts{hostname=""} 5942
# HELP ceems_ipmi_dcmi_current_watts Current Power consumption in watts
# TYPE ceems_ipmi_dcmi_current_watts gauge
ceems_ipmi_dcmi_current_watts{hostname=""} 5942
# HELP ceems_ipmi_dcmi_max_watts Maximum Power consumption in watts
# TYPE ceems_ipmi_dcmi_max_watts gauge
ceems_ipmi_dcmi_max_watts{hostname=""} 6132
# HELP ceems_ipmi_dcmi_min_watts Minimum Power consumption in watts
# TYPE ceems_ipmi_dcmi_min_watts gauge
ceems_ipmi_dcmi_min_watts{hostname=""} 5748
# HELP ceems_meminfo_MemAvailable_bytes Memory information field MemAvailable_bytes.
# TYPE ceems_meminfo_MemAvailable_bytes gauge
ceems_meminfo_MemAvailable_bytes{hostname=""} 0
# HELP ceems_meminfo_MemFree_bytes Memory information field MemFree_bytes.
# TYPE ceems_meminfo_MemFree_bytes gauge
ceems_meminfo_MemFree_bytes{hostname=""} 4.50891776e+08
# HELP ceems_meminfo_MemTotal_bytes Memory information field MemTotal_bytes.
# TYPE ceems_meminfo_MemTotal_bytes gauge
ceems_meminfo_MemTotal_bytes{hostname=""} 1.6042172416e+10
# HELP ceems_rapl_package_joules_total Current RAPL package value in joules
# TYPE ceems_rapl_package_joules_total counter
ceems_rapl_package_joules_total{hostname="",index="0",path="pkg/collector/testdata/sys/class/powercap/intel-rapl:0"} 258218.293244
ceems_rapl_package_joules_total{hostname="",index="1",path="pkg/collector/testdata/sys/class/powercap/intel-rapl:1"} 130570.505826
```

If the `ceems_exporter` process have necessary capabilities assigned either _via_ 
file capabilities or process capabilities, the flags `--collector.slurm.job.props.path` 
and `--collector.slurm.gpu.job.map.path` can be omitted and there is no need to 
set up prolog and epilog scripts.

### `ceems_api_server`

The stats server can be started as follows:

```
/path/to/ceems_api_server \
    --resource.manager.slurm \
    --storage.data.path="/var/lib/ceems" \
    --log.level="debug"
```

Data files like SQLite3 DB created for the server will be placed in 
`/var/lib/ceems` directory. Note that if this directory does exist, 
`ceems_api_server` will attempt to create one if it has enough privileges. If it 
fails to create, error will be shown up.

<!-- To execute `sacct` command as `slurm` user, command becomes following:

```
/path/to/ceems_api_server \
    --slurm.sacct.path="/usr/local/bin/sacct" \
    --slurm.sacct.run.as.slurmuser \
    --path.data="/var/lib/ceems" \
    --log.level="debug"
```

Note that this approach needs capabilities assigned to process. On the other hand, if 
we want to use `sudo` approach to execute `sacct` command, the command becomes:

```
/path/to/ceems_api_server \
    --slurm.sacct.path="/usr/local/bin/sacct" \
    --slurm.sacct.run.with.sudo \
    --path.data="/var/lib/ceems" \
    --log.level="debug"
```

This requires an entry into sudoers file that permits the user starting 
`ceems_api_server` to execute `sudo sacct` without password. -->

`ceems_api_server` updates the local DB with job information regularly. The frequency 
of this update and period for which the data will be retained can be configured
too. For instance, the following command will update the DB for every 30 min and 
keeps the data for the past one year.

```
/path/to/ceems_api_server \
    --resource.manager.slurm \
    --storage.path.data="/var/lib/ceems" \
    --storage.data.update.interval="30m" \
    --storage.data.retention.period="1y" \
    --log.level="debug"
```

### `ceems_lb`

A basic config file used by `ceems_lb` is as follows:

```
strategy: resource-based
db_path: data/ceems_api_server.db
backends:
  - url: "http://localhost:9090"
  - url: "http://localhost:9091"
```

- Keyword `strategy` can take either `round-robin`, `least-connection` and `resource-based` 
as values. Using `resource-based` strategy, the queries are proxied to backend TSDB 
instances based on the data available with each instance as 
[described in CEEMS load balancer](#ceems-load-balancer).
- Keyword `db_path` takes the path to CEEMS API server DB file. This file is optional and
if provided, it offers basic access control
- Keyword `backends` take a list of TSDB backends.

The load balancer can be started as follows:

```
/path/to/cemms_lb \
    --config.file=config.yml \
    --log.level="debug"
```

This will start a load balancer at port `9030` by default. In Grafana we need to 
configure this load balancer as Prometheus data source URL as requests are proxied by
the load balancer.

## TLS and basic auth

Exporter and API server support TLS and basic auth using 
[exporter-toolkit](https://github.com/prometheus/exporter-toolkit). To use TLS and/or 
basic auth, users need to use `--web-config-file` CLI flag as follows

```
ceems_exporter --web-config-file=web-config.yaml
ceems_api_server --web-config-file=web-config.yaml
ceems_lb --web-config-file=web-config.yaml
```

A sample `web-config.yaml` file can be fetched from 
[exporter-toolkit repository](https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-config.yml). 
The reference of the `web-config.yaml` file can be consulted in the 
[docs](https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md).
