# Philosophy

## Supported metrics

### CPU, memory, IO and network

The idea we are leveraging here is that every resource manager has to resort to cgroups
on Linux to manage CPU, memory, and IO resources. Each resource manager does it
differently, but the takeaway here is that the accounting information is readily
available in the cgroups. By walking through the cgroups file system, we can gather the
metrics and map them to a particular compute unit as resource managers tend to create
cgroups for each compute unit with some sort of identifier attached to it.

Although cgroups already provide us with rich information about metrics of individual
compute units, some important metrics are still unavailable at the cgroup level, notably
IO and network metrics. It is worth noting that controllers do exist in cgroups for
network and IO, but they
do not cover all real-world cases. For instance, the IO controller can be used only
with block devices, whereas many real-world applications rely on network file systems.
Node-level network and IO metrics can be gathered from `/sys` and `/proc` file systems
relatively easily, however, the challenge here is to monitor those metrics at the cgroup
level.

CEEMS uses the [eBPF](https://ebpf.io/what-is-ebpf/) framework to monitor network and IO
metrics. CEEMS loads BPF programs
that trace several kernel functions that are in the data path for both IO and network and exposes
those metrics for each cgroup. More importantly, this is done in a way that is
agnostic to the resource manager and underlying file system. Similarly, network metrics for
TCP and UDP protocols for both IPv4 and IPv6 can be gathered by using carefully crafted
BPF programs and attaching to relevant kernel functions.

This is a distributed approach where a daemon Prometheus exporter will run on
each compute node. Whenever Prometheus makes a scrape request, the exporter will
walk through the cgroup file system and BPF program maps and
expose the data to Prometheus. As reading the cgroups file system is relatively cheap,
there is very little overhead in running this daemon service. Similarly, BPF programs are
extremely fast and efficient as they are executed in kernel space. On average, the exporter
takes less than 20 MB of memory.

### Energy metrics

In an age where green computing is becoming more and more important, it is essential to
expose the energy consumed by the compute units to users to make them more aware.
Most energy measurement tools are based on
[RAPL](https://www.kernel.org/doc/html/next/power/powercap/powercap.html), which reports
energy consumption from CPU and memory. It does not report consumption from other
peripherals like PCIe, network, disk, _etc_.

To address this, the current exporter will expose node power statistics from BMC
via IPMI/Redfish/Cray's PM counters in addition to
RAPL metrics. BMC measurements are generally made at the node level, which includes
consumption by _most_ of the components. However, the reported energy usage is vendor
dependent, and it is desirable to validate with them before drawing conclusions from the
numbers. In any case, this is the only complete metric we can get our hands on without
needing to install any additional hardware like wattmeters.

This node-level power consumption can be split into consumption of individual compute units
by using relative CPU times used by the compute unit. Although this is not an exact
estimation of power consumed by the compute unit, it remains a very good approximation.

### Emission metrics

The exporter is capable of exporting emission factors from different data sources
which can be used to estimate equivalent CO2 emissions. Currently, for
France, a _real-time_ emission factor will be used that is based on
[RTE eCO2 mix data](https://www.rte-france.com/en/eco2mix/co2-emissions). Besides,
retrieving emission factors from [Electricity Maps](https://app.electricitymaps.com/map)
is also supported, provided that an API token is provided. Electricity Maps provide
emission factor data for most countries. Similarly, factors from [Watt Time](https://watttime.org/)
can also be used when account credentials are available. A static emission factor from historic
data is also provided from [OWID data](https://github.com/owid/co2-data). Finally, a
constant global average emission factor can also be used.

The emissions collector is capable of exporting emission factors from different sources,
and users can choose the factor that suits their needs.

### GPU metrics

Currently, only NVIDIA and AMD GPUs are supported. This exporter leverages
[DCGM exporter](https://github.com/NVIDIA/dcgm-exporter/tree/main) for NVIDIA GPUs and
[AMD SMI exporter](https://github.com/amd/amd_smi_exporter) for AMD GPUs to get GPU metrics of
each compute unit. DCGM/AMD SMI exporters expose the GPU metrics of each GPU, and the
current exporter takes care of the GPU index to compute unit mapping. These two metrics
can be used together using PromQL to show the metrics of GPU metrics of a given compute
unit.

In the case of vGPUs supported by NVIDIA Grid, the energy consumed by each vGPU is
estimated using the total energy consumption of the physical GPU and the number of active
vGPUs scheduled on that physical GPU. Similarly, in the case of Multi-Instance GPU (MIG),
the energy consumption of each MIG instance is estimated based on the relative number
of Streaming Multiprocessors (SM) and total energy consumption of the physical GPU.

### Performance metrics

Presenting energy and emission metrics is only one side of the story. This will
help end users to quickly and cheaply identify their workloads that are consuming
a lot of energy. However, to make those workloads more efficient, we need more
information about the application _per se_. To address this CEEMS exposes performance
related metrics fetched from Linux's [perf](https://perf.wiki.kernel.org/index.php/Main_Page)
subsystem. These metrics help end users to quickly identify the obvious issues with
their applications and thereby improve them and eventually make them more
energy efficient.

Currently, CEEMS provides performance metrics for CPU. It is possible to gather
performance metrics for NVIDIA GPUs as well, as long as operators install and enable
NVIDIA DCGM libraries. More details can be found in the
[DCGM](https://docs.nvidia.com/datacenter/dcgm/latest/user-guide/feature-overview.html#profiling-metrics)
documentation.

### Continuous Profiling

[Continuous Profiling](https://www.cncf.io/blog/2022/05/31/what-is-continuous-profiling/) enables
users to profile their code on production systems, which can help them fix abnormal CPU
usage, memory leaks, etc. A good primer for continuous profiling can be consulted from
[Elastic Docs](https://www.elastic.co/what-is/continuous-profiling). The CEEMS stack lets users
and developers identify which applications or processes to continuously profile and works in tandem with profiling software to monitor these targets.

## Technologies involved

### Databases

One of the principal objectives of the CEEMS stack is to avoid creating new software and use
open-source components as much as possible. It is clear that the stack needs a Time Series
Database (TSDB) to store time series metrics of compute units, and [Prometheus](https://prometheus.io/)
has proven to be the _de facto_ standard in the cloud-native community for its performance. Thus,
CEEMS uses Prometheus (or PromQL-compliant TSDB) as its TSDB. CEEMS also uses a relational
DB for storing a list of compute units along with their aggregate metrics from different
resource managers. CEEMS uses [SQLite](https://www.sqlite.org/) for its simplicity and
performance. Moreover, CEEMS' relational DB does not need concurrent writes as there is always
a single thread (goroutine) fetching compute units from the underlying resource manager
and writing them to the DB. Thus, SQLite can be a very good option and avoids having to
maintain complex DB servers.

For the case of continuous profiling, [Grafana Pyroscope](https://grafana.com/oss/pyroscope/)
provides an OSS version of a continuous profiling database, which can be regarded as equivalent
to Prometheus for profiling data. [Grafana Alloy](https://grafana.com/docs/alloy/latest/)
is the agent that runs on all compute nodes like the Prometheus exporter, which in turn sends
profiling data to the Pyroscope server. The CEEMS stack provides a list of targets (processes)
that need continuous profiling to Grafana Alloy.

### Visualization

Once the metrics are gathered, we need an application to visualize metrics for end users
in a user-friendly way. CEEMS uses [Grafana](https://grafana.com/grafana/), which is also
the _de facto_ standard in the cloud-native community. Grafana has very good integration for
Prometheus and also for Grafana Pyroscope.
