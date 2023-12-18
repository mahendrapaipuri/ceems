# Batch job metrics monitoring stack

This repository contains a Prometheus exporter to export job metrics of batch jobs 
like SLURM, PBS, LSF, _etc_ and other utility tools that can be used to deploy a 
fully functional monitoring stack on a HPC platform.

## Design objectives

### CPU, memory and IO metrics

The main design objective of this stack is to gathering job metrics _via_ cgroups and 
avoid using batch scheduler native tools like `sacct` for SLURM. The rationale is that 
on huge HPC platforms (nodes > 2000), that churn few thousands of jobs at a given 
time, gathering time series job metrics from a tool like `sacct`, say every 10 sec, can 
put a lot of stress on DB which can negatively impact the performance of batch scheduler.

The idea we are leveraging here is that every resource manager has to resort to cgroups 
on Linux to manage the quota on CPU, memory and IO. Each resource manager does it 
differently but the take away here is that the accounting information is readily 
available in the cgroups. By walking through the cgroups file system, we can gather the 
job metrics that map them to a particular job as resource manager tends to create 
cgroups for each job with some sort of job identifier attached to it.

This is a distributed approach where exporter will run on each compute node and walk 
through cgroup file system whenever Prometheus make a scrape request and pull the data 
to Prometheus. As reading cgroups file system is relatively cheap, there is a very 
little overhead running this daemon service.  

### Energy consumption

In an age where green computing is becoming more and more important, it is essential to
expose the energy consumed by the batch jobs to the users to make them more aware. 
Most of energy measurement tools are based on 
[RAPL](https://www.kernel.org/doc/html/next/power/powercap/powercap.html) which reports 
mostly CPU and memory consumption. It does not report consumption from other peripherals 
like PCIe, network, disk, _etc_. 

To address this, the current exporter will expose IPMI power statistics in addition to 
RAPL metrics. IPMI measurements are generally made at the node level which includes 
consumption by _most_ of the components. However, the implementations are vendor 
dependent and it is desirable to validate with them before reading too much into the 
numbers. In any case, this is the only complete metric we can get our hands on without 
needing to install any additional hardware like Wattmeters. 

This monitoring power consumption can be split into consumption of individual batch jobs
by using relative CPU times used by batch job. Although, this is not an exact 
estimation of power consumed by the batch job, it stays a very good approximation.

### Emissions

The exporter is capable of exporting emission factor data which can be used in 
conjunction energy consumption to estimate equivalent CO2 emissions. Currently, for 
France, a _real_ time emission factor will be used that is based on 
[RTE eCO2 mix data](https://www.rte-france.com/en/eco2mix/co2-emissions). For other 
countries, a constant average based on historic data will be used. This historic data 
is gathered from [CodeCarbon's DB](https://raw.githubusercontent.com/mlco2/codecarbon/master/codecarbon/data/private_infra/global_energy_mix.json).

### GPU metrics

Currently, only nVIDIA GPUs are supported. This exporter leverages 
[DCGM exporter](https://github.com/NVIDIA/dcgm-exporter/tree/main) to get GPU metrics of
each job. The current exporter only exposes the GPU index to job mapping which will be 
used in Grafana dashboard to show GPU metrics of each job.

## Repository contents

This monorepo contains three main utils that are essential for the batch job monitoring 
stack.

- `batchjob_exporter`: This is the Prometheus exporter that exposes individual job 
metrics, RAPL energy, IPMI power consumption, emission factor and GPU to batch job 
mapping.

- `batchjob_stats_server`: This is a simple API server that exposes accounts and jobs 
information of users by querying a SQLite3 DB. 
This server will be used as 
[JSON API DataSource](https://grafana.github.io/grafana-json-datasource/installation/) 
in Grafana to construct dashboards for users. The DB will be updated in a separate go 
routine that queries job statistics from the underlying batch scheduler at a configured 
interval. In the case of SLURM, it is `sacct` command that pulls job statistics and 
populate the local DB.

## Getting started

### Install

Pre-compiled binaries of the apps can be downloaded from the 
[releases](https://github.com/mahendrapaipuri/batchjob_monitoring/releases/).

### Build

As the `batchjob_stats_server` uses SQLite3 as DB backend, we are dependent on CGO for 
compiling that app. On the other hand, `batchjob_exporter` is a pure GO application. 
Thus, in order to build from sources, users need to execute two build commands

```
make build
```

that builds `batchjob_exporter` binary and

```
CGO_BUILD=1 make build
```

which builds `batchjob_stats_server` app.

Both of them will be placed in `bin` folder in the root of the repository

### Running tests

In the same way, to run unit and end-to-end tests for apps, it is enough to run

```
make tests
CGO_BUILD=1 make tests
```

## Configuration

Currently, the exporter supports only SLURM. `batchjob_exporter` provides following collectors:

- Slurm collector: Exports SLURM job metrics like CPU, memory and IO usage
- nVIDIA GPU collector: Exports GPU indices to job ID maps
- IPMI collector: Exports power usage reported by `ipmi` tools
- RAPL collector: Exports RAPL energy metrics
- Emissions collector: Exports emission factor (g eCO2/kWh)

### Slurm collector

As batch job schedulers tend to reset 
job IDs after certain overflow, it is desirable 
to have a _unique_ job ID during these resets. This constraint is more important on 
big HPC platforms that have high churn of batch jobs. `cgroups` created by SLURM do not 
have any information on job except for its job ID. Hence, we need to get few more 
job properties to calculate a unique job ID. 

Currently the exporter supports few different ways to get these job properties.

- Prolog and epilog: Using SLURM prolog and epilog scripts that writes these job 
properties to a file which can be read by the exporter. Thee scripts will create a file 
named after job ID with required properties as its content delimited by space. 
Similarly, for GPU to job maps, each GPU will create a file with its index as file name
and writes the job ID in the file. In the epilog scripts, that created files will be 
removed. Example [prolog and epilog scripts](./etc/slurm) are provided in the repo. 
This approach requires the `batchjob_exporter` to be configured with command line 
option `--collector.slurm.job.props.path=/run/slurmjobprops` assuming the files with 
slurm job properties are being written in `/run/slurmjobprops` directory.

- Reading env vars from `/proc`: If the file created by prolog script cannot be found, 
the exporter defaults to reading the `/proc` file system and attempt to job properties
by reading environment variables of processes. However, this needs privileges which 
can be attributed by assigning `CAP_SYS_PTRACE` and `CAP_DAC_READ_SEARCH` capabilities 
to the `batchjob_exporter` process. Assigning capabilities to process is discussed 
in [capabilities section](#linux-capabilities).

- Running exporter as `root`: This will assign all available capabilities for the 
`batchjob_exporter` process and thus the necessary job properties and GPU maps will be
read from environment variables in `/proc` file system.

It is recommended to use Prolog and Epilog scripts to get job properties and GPU to job ID maps 
as it does not require any privileges and exporter can run completely in the 
userland. If the admins would not want to have the burden of maintaining prolog and 
epilog scripts, it is better to assign capabilities. These two approaches should be always favoured to running the exporter as `root`. 

### nVIDIA GPU job map collector

This collector exports the GPU ordinal index to job ID map to Prometheus. The actual 
GPU metrics are exported using [dcgm-exporter](https://github.com/NVIDIA/dcgm-exporter). 
Like in the case of SLURM collector, we need to know which GPU is allocated to which 
job and this info is not available post job. Thus, similar approaches as SLURM collector 
are available for this collector too.

- Use prolog and epilog scripts to get the GPU to job ID map. Example prolog script 
is provided in the [repo](./etc/slurm/prolog.d/gpujobmap.sh). Similarly, this approach 
needs `--collector.nvidia.gpu.job.map.path=/run/gpujobmap` command line option.

- Using capabilities to read the environment variables directly from `/proc` file system.

- Running exporter as `root`.

### IPMI collector

There are several IPMI implementation available like FreeIPMI, IPMITool, IPMIUtil, 
OpenIPMI, _etc._ Current exporter allows to configure the IPMI command that will report 
the power usage of the node. The default value is set to FreeIPMI one as 
`--collector.ipmi.dcmi.cmd="/usr/bin/ipmi-dcmi --get-system-power-statistics"`. The 
output of the command expects following lines:

```
Current Power                        : 332 Watts
Power Measurement                    : Active
```

If your IPMI implementation does not return an output like above, you can write your 
own wrapper that parses your IPMI implementation's output and returns output in above 
format. 

Generally `ipmi` related commands are available for only `root`. Admins can add a sudoers 
entry to let the user that runs the `batchjob_exporter` to execute only necessary 
command that reports the power usage. For instance, in the case of FreeIPMI 
implementation, that sudoers entry will be

```
batchjob-exporter ALL = NOPASSWD: /usr/sbin/ipmi-dcmi
```

and pass the flag `--collector.ipmi.dcmi.cmd="sudo /usr/bin/ipmi-dcmi --get-system-power-statistics"` 
to `batchjob_exporter`.

Another supported approach is to run the subprocess `ipmi-dcmi` command as root. In this 
approach, the subprocess will be spawned as root to be able to execute the command. 
This needs `CAP_SETUID` and `CAP_SETGID` capabilities in order to able use `setuid` and
`setgid` syscalls.

### RAPL collector

For the kernels that are `<5.3`, there is no special configuration to be done. If the 
kernel version is `>=5.3`, RAPL metrics are only available for `root`. The capability 
`CAP_DAC_READ_SEARCH` should be able to circumvent this restriction although this has 
not been tested. Another approach is to add a ACL rule on the `/sys/fs/class/powercap` 
directory to give read permissions to the user that is running `batchjob_exporter`.

### Batch Job Stats API server

As discussed in the [introduction](#batch-job-stats-api-server), `batchjob_stats_server` 
exposes accounts and jobs details of users _via_ API end points. This data will be 
gathered from the underlying batch scheduler at a configured interval of time and 
keep it in a local DB. In the case of SLURM, the app executes `sacct` command to get 
info on jobs. However, `sacct` command needs to be executed as either `root` or `slurm` 
user to get job details of _all_ users. 

Current implementation supports two approaches for this privileged action

- Spawn a subprocess for `sacct` and execute it as `slurm` user by using 
`CAP_SETUID` and `CAP_SETGID` capabilities. This approach can be used by passing flag 
`--slurm.sacct.run.as.slurmuser`.

- Run `sacct` with `sudo`. This required that we need to give the user permission to use 
`sudo` by adding an entry into sudoers file. This approach can be used by passing 
`--slurm.sacct.run.with.sudo`.

Above stated approaches are mutually exclusive. If attempted to use both of them at the 
same time, server will not start by returning an error.

## Linux capabilities

Linux capabilities can be assigned to either file or process. For instance, capabilities 
on the `batchjob_exporter` and `batchjob_stats_server` binaries can be set as follows:

```
sudo setcap cap_sys_ptrace,cap_dac_read_search,cap_setuid,cap_setgid+ep /full/path/to/batchjob_exporter
sudo setcap cap_setuid,cap_setgid+ep /full/path/to/batchjob_stats_server
```

This will assign all the capabilities that are necessary to run `batchjob_exporter` 
for all the collectors stated in the above section. Using file based capabilities will 
expose those capabilities to anyone on the system that have execute permissions on the 
binary. Although, it does not pose a big security concern, it is better to assign 
capabilities to a process. 

As admins tend to run the exporter within a `systemd` unit file, we can assign 
capabilities to the process rather than file using `AmbientCapabilities` 
directive of the `systemd`. An example is as follows:

```
[Service]
ExecStart=/usr/local/bin/batchjob_exporter
AmbientCapabilities=CAP_SYS_PTRACE CAP_DAC_READ_SEARCH CAP_SETUID CAP_SETGID
```

Note that it is bare minimum service file and it is only to demonstrate on how to use 
`AmbientCapabilities`. Production ready service files examples are provided in 
[repo](./etc/systemd)

## Usage

### `batchjob_exporter`

Using prolog and epilog scripts approach and `sudo` for `ipmi`, 
`batchjob_exporter` can be started as follows

```
/path/to/batchjob_exporter \
    --collector.slurm.job.props.path="/run/slurmjobprops" \
    --collector.ipmi.dcmi.cmd="sudo /usr/sbin/ipmi-dcmi --get-system-power-statistics" \
    --collector.nvidia_gpu \
    --collector.nvidia.gpu.job.map.path="/run/gpujobmap" \
    --log.level="debug"
```

This will start exporter server on default 9010 port. Metrics can be consulted using 
`curl http://localhost:9010/metrics` command which will give an output as follows:

```
# HELP batchjob_cpu_system_seconds Cumulative CPU system seconds
# TYPE batchjob_cpu_system_seconds gauge
batchjob_cpu_system_seconds{batch="slurm",jobaccount="testacc",jobid="1009248",jobuuid="ac28caf5-ce6c-35f6-73fb-47d9d43f7780",step="",task=""} 115.777502
# HELP batchjob_cpu_total_seconds Cumulative CPU total seconds
# TYPE batchjob_cpu_total_seconds gauge
batchjob_cpu_total_seconds{batch="slurm",jobaccount="testacc",jobid="1009248",jobuuid="ac28caf5-ce6c-35f6-73fb-47d9d43f7780",step="",task=""} 60491.070351
# HELP batchjob_cpu_user_seconds Cumulative CPU user seconds
# TYPE batchjob_cpu_user_seconds gauge
batchjob_cpu_user_seconds{batch="slurm",jobaccount="testacc",jobid="1009248",jobuuid="ac28caf5-ce6c-35f6-73fb-47d9d43f7780",step="",task=""} 60375.292848
# HELP batchjob_cpus Number of CPUs
# TYPE batchjob_cpus gauge
batchjob_cpus{batch="slurm",jobaccount="testacc",jobid="1009248",jobuuid="ac28caf5-ce6c-35f6-73fb-47d9d43f7780",step="",task=""} 2
# HELP batchjob_exporter_build_info A metric with a constant '1' value labeled by version, revision, branch, goversion from which batchjob_exporter was built, and the goos and goarch for the build.
# TYPE batchjob_exporter_build_info gauge
batchjob_exporter_build_info{branch="main",goarch="amd64",goos="linux",goversion="go1.21.3",revision="50a5db3888711a35341891a2bdd4925549ad6a14",tags="netgo osusergo static_build",version="Unreleased"} 1
# HELP batchjob_ipmi_dcmi_watts_total Current Power consumption in watts
# TYPE batchjob_ipmi_dcmi_watts_total counter
batchjob_ipmi_dcmi_watts_total 332
# HELP batchjob_memory_cache_bytes Memory cache used in bytes
# TYPE batchjob_memory_cache_bytes gauge
batchjob_memory_cache_bytes{batch="slurm",jobaccount="testacc",jobid="1009248",jobuuid="ac28caf5-ce6c-35f6-73fb-47d9d43f7780",step="",task=""} 0
# HELP batchjob_memory_fail_count Memory fail count
# TYPE batchjob_memory_fail_count gauge
batchjob_memory_fail_count{batch="slurm",jobaccount="testacc",jobid="1009248",jobuuid="ac28caf5-ce6c-35f6-73fb-47d9d43f7780",step="",task=""} 0
# HELP batchjob_memory_rss_bytes Memory RSS used in bytes
# TYPE batchjob_memory_rss_bytes gauge
batchjob_memory_rss_bytes{batch="slurm",jobaccount="testacc",jobid="1009248",jobuuid="ac28caf5-ce6c-35f6-73fb-47d9d43f7780",step="",task=""} 4.098592768e+09
# HELP batchjob_memory_total_bytes Memory total in bytes
# TYPE batchjob_memory_total_bytes gauge
batchjob_memory_total_bytes{batch="slurm",jobaccount="testacc",jobid="1009248",jobuuid="ac28caf5-ce6c-35f6-73fb-47d9d43f7780",step="",task=""} 4.294967296e+09
# HELP batchjob_memory_used_bytes Memory used in bytes
# TYPE batchjob_memory_used_bytes gauge
batchjob_memory_used_bytes{batch="slurm",jobaccount="testacc",jobid="1009248",jobuuid="ac28caf5-ce6c-35f6-73fb-47d9d43f7780",step="",task=""} 4.111491072e+09
# HELP batchjob_memsw_fail_count Swap fail count
# TYPE batchjob_memsw_fail_count gauge
batchjob_memsw_fail_count{batch="slurm",jobaccount="testacc",jobid="1009248",jobuuid="ac28caf5-ce6c-35f6-73fb-47d9d43f7780",step="",task=""} 0
# HELP batchjob_memsw_total_bytes Swap total in bytes
# TYPE batchjob_memsw_total_bytes gauge
batchjob_memsw_total_bytes{batch="slurm",jobaccount="testacc",jobid="1009248",jobuuid="ac28caf5-ce6c-35f6-73fb-47d9d43f7780",step="",task=""} 0
# HELP batchjob_memsw_used_bytes Swap used in bytes
# TYPE batchjob_memsw_used_bytes gauge
batchjob_memsw_used_bytes{batch="slurm",jobaccount="testacc",jobid="1009248",jobuuid="ac28caf5-ce6c-35f6-73fb-47d9d43f7780",step="",task=""} 0
# HELP batchjob_rapl_package_joules_total Current RAPL package value in joules
# TYPE batchjob_rapl_package_joules_total counter
batchjob_rapl_package_joules_total{index="0",path="pkg/collector/fixtures/sys/class/powercap/intel-rapl:0"} 258218.293244
batchjob_rapl_package_joules_total{index="1",path="pkg/collector/fixtures/sys/class/powercap/intel-rapl:1"} 130570.505826
# HELP batchjob_scrape_collector_duration_seconds batchjob_exporter: Duration of a collector scrape.
# TYPE batchjob_scrape_collector_duration_seconds gauge
batchjob_scrape_collector_duration_seconds{collector="ipmi_dcmi"} 0.003479042
batchjob_scrape_collector_duration_seconds{collector="nvidia_gpu"} 1.66e-05
batchjob_scrape_collector_duration_seconds{collector="rapl"} 0.001222098
batchjob_scrape_collector_duration_seconds{collector="slurm_job"} 0.005055937
# HELP batchjob_scrape_collector_success batchjob_exporter: Whether a collector succeeded.
# TYPE batchjob_scrape_collector_success gauge
batchjob_scrape_collector_success{collector="ipmi_dcmi"} 1
batchjob_scrape_collector_success{collector="nvidia_gpu"} 1
batchjob_scrape_collector_success{collector="rapl"} 1
batchjob_scrape_collector_success{collector="slurm_job"} 1
```

If the `batchjob_exporter` process have necessary capabilities assigned either _via_ 
file capabilities or process capabilities, the flags `--collector.slurm.job.props.path` 
and `--collector.nvidia.gpu.job.map.path` can be omitted and there is no need to 
set up prolog and epilog scripts.

### `batchjob_stats_server`

The stats server can be started as follows:

```
/path/to/batchjob_stats_server \
    --slurm.sacct.path="/usr/local/bin/sacct" \
    --path.data="/var/lib/batchjob_stats" \
    --log.level="debug"
```

Data files like SQLite3 DB created for the server will be placed in 
`/var/lib/batchjob_stats` directory. Note that this directory needs to be created with 
proper permissions before starting the server. 

To execute `sacct` command as `slurm` user, command becomes following:

```
/path/to/batchjob_stats_server \
    --slurm.sacct.path="/usr/local/bin/sacct" \
    --slurm.sacct.run.as.slurmuser \
    --path.data="/var/lib/batchjob_stats" \
    --log.level="debug"
```

Note that this approach needs capabilities assigned to process. On the other hand, if 
we want to use `sudo` approach to execute `sacct` command, the command becomes:

```
/path/to/batchjob_stats_server \
    --slurm.sacct.path="/usr/local/bin/sacct" \
    --slurm.sacct.run.with.sudo \
    --path.data="/var/lib/batchjob_stats" \
    --log.level="debug"
```

This requires an entry into sudoers file that permits the user starting 
`batchjob_stats_server` to execute `sudo sacct` without password.

`batchjob_stats_server` updates the local DB with job information regularly. The frequency 
of this update and period for which the job data will be retained can be configured
too. For instance, the following command will update the job DB for every 30 min and 
keeps the job data for the past one year.

```
/path/to/batchjob_stats_server \
    --slurm.sacct.path="/usr/local/bin/sacct" \
    --slurm.sacct.run.with.sudo \
    --path.data="/var/lib/batchjob_stats" \
    --db.update.interval=1800 \
    --data.retention.period=365 \
    --log.level="debug"
```

## TLS and basic auth

Exporter and API server support TLS and basic auth using 
[exporter-toolkit](https://github.com/prometheus/exporter-toolkit). To use TLS and/or 
basic auth, users need to use `--web-config-file` CLI flag as follows

```
batchjob_exporter --web-config-file=web-config.yaml
batchjob_stats_server --web-config-file=web-config.yaml
```

A sample `web-config.yaml` file can be fetched from 
[exporter-toolkit repository](https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-config.yml). 
The reference of the `web-config.yaml` file can be consulted in the 
[docs](https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md).
