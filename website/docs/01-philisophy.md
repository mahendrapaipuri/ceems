# Philosophy

## CPU, memory, IO and network metrics

The idea we are leveraging here is that every resource manager has to resort to cgroups
on Linux to manage CPU, memory and IO resources. Each resource manager does it
differently but the take away here is that the accounting information is readily
available in the cgroups. By walking through the cgroups file system, we can gather the
metrics that map them to a particular compute unit as resource manager tends to create
cgroups for each compute unit with some sort of identifier attached to it.

Although cgroups already provide us with rich information about metrics of individual
compute units, some important metrics are still unavailable at cgroup level notably
IO and network metrics. It is worth noting that controllers do exist in cgroups for
network and IO but they
do not cover all the realworld cases. For instace the IO controller can be used only
with block devices whereas a lot of realworld applications rely on network file systems.
Node-level network and IO metrics can be gathered from `/sys` and `/proc` file systems
relatively easily, however, the challenge here is to monitor those metrics at the cgroup
level.

CEEMS use [eBPF](https://ebpf.io/what-is-ebpf/) framework to monitor network and IO
metrics. CEEMS loads bpf programs
that trace several kernel functions that are in the data path for both IO and network and expose
those metrics for each cgroup. More importantly, this is done in a way that it is
agnostic to resource manager and underlying file system. Similarly network metrics for
TCP and UDP protocols for both IPv4 and IPv6 can be gathered by using carefully crafted
bpf programs and attaching to relevant kernel functions.

This is a distributed approach where a daemon exporter will run on each compute node. Whenever
Prometheus make a scrape request, the exporter will walk through cgroup file system and
bpf program maps and
exposes the data to Prometheus. As reading cgroups file system is relatively cheap,
there is a very little overhead running this daemon service. Similarly, BPF programs are
extremely fast and efficient as they are run in kernel space. On average the exporter
takes less than 20 MB of memory.

## Energy consumption

In an age where green computing is becoming more and more important, it is essential to
expose the energy consumed by the compute units to the users to make them more aware.
Most of energy measurement tools are based on
[RAPL](https://www.kernel.org/doc/html/next/power/powercap/powercap.html) which reports
energy consumption from CPU and memory. It does not report consumption from other
peripherals like PCIe, network, disk, _etc_.

To address this, the current exporter will expose IPMI power statistics in addition to
RAPL metrics. IPMI measurements are generally made at the node level which includes
consumption by _most_ of the components. However, the reported energy usage is vendor
dependent and it is desirable to validate with them before reading too much into the
numbers. In any case, this is the only complete metric we can get our hands on without
needing to install any additional hardware like Wattmeters.

This node level power consumption can be split into consumption of individual compute units
by using relative CPU times used by the compute unit. Although, this is not an exact
estimation of power consumed by the compute unit, it stays a very good approximation.

## Emissions

The exporter is capable of exporting emission factors from different data sources
which can be used to estimate equivalent CO2 emissions. Currently, for
France, a _real_ time emission factor will be used that is based on
[RTE eCO2 mix data](https://www.rte-france.com/en/eco2mix/co2-emissions). Besides,
retrieving emission factors from [Electricity Maps](https://app.electricitymaps.com/map)
is also supported provided that API token is provided. Electricity Maps provide
emission factor data for most of the countries. A static emission factor from historic
data is also provided from [OWID data](https://github.com/owid/co2-data). Finally, a
constant global average emission factor can also be used.

Emissions collector is capable of exporting emission factors from different sources
and users can choose the factor that suits their needs.

## GPU metrics

Currently, only nVIDIA and AMD GPUs are supported. This exporter leverages
[DCGM exporter](https://github.com/NVIDIA/dcgm-exporter/tree/main) for nVIDIA GPUs and
[AMD SMI exporter](https://github.com/amd/amd_smi_exporter) for AMD GPUs to get GPU metrics of
each compute unit. DCGM/AMD SMI exporters exposes the GPU metrics of each GPU and the
current exporter takes care of the GPU index to compute unit mapping. These two metrics
can be used together using PromQL to show the metrics of GPU metrics of a given compute
unit.

## Performance metrics

Presenting energy and emission metrics is only one side of the story. This will
help end users to quickly and cheaply identify their workloads that are consuming
a lot of energy. However, to make those workloads more efficient, we need more
information about the application _per se_. To address this CEEMS exposes performance
related metrics fetched from Linux's [perf](https://perf.wiki.kernel.org/index.php/Main_Page)
subsystem. These metrics help end users to quickly identify the obvious issues with
their applications and there by improving them and eventually making them more
energy efficient.

Currently, CEEMS provides performance metrics for CPU. It is possible to gather
performance metrics for nVIDIA GPUs as well as long as operators install and enable
nVIDIA DCGM libraries. More details can be found in [DCGM](https://docs.nvidia.com/datacenter/dcgm/latest/user-guide/feature-overview.html#profiling-metrics)
docs.
