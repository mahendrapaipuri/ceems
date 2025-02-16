---
sidebar_position: 1
---

# CEEMS Exporter

## Basic usage

:::important[IMPORTANT]

Currently CEEMS exporter supports exporting SLURM job and Openstack VM metrics.
Adding support for k8s is in next milestone.

:::

To run exporter with default enabled collectors, use the following command:

```bash
ceems_exporter 
```

List of collectors that are enabled by default are:

- `cpu`: Node level CPU stats
- `memory`: Node level memory stats
- `rapl`: RAPL energy counters

By default CEEMS exporter exposes metrics on all interfaces, port `9010` and
at `/metrics` endpoint. This can be changed by setting `--web.listen-address` CLI flag

```bash
ceems_exporter --web.listen-address="localhost:8010"
```

Above command will run exporter only on `localhost` and on port `8010`.

:::tip[TIP]

All the available command line options are listed in
[CEEMS Exporter CLI docs](../cli/ceems-exporter.md).

:::

In order to enable SLURM collector, we need to add the following CLI flag

```bash
ceems_exporter --collector.slurm
```

:::important[IMPORTANT]

Starting from `v0.3.0`, there is no need to configure the GPU type. The exporter will
automatically detect the supported GPU types: NVIDIA and AMD.

:::

In order to disable default collectors, we need to add `no` prefix to the collector flag.
The following command will disable IPMI and RAPL collectors:

```bash
ceems_exporter --no-collector.rapl
```

By default no authentication is imposed on the exporter web server. In production this
is no advisable and it is possible to add basic auth and TLS to the exporter using
a web configuration file. More details on how to setup web configuration is discussed
in [Web configuration](../configuration/basic-auth.md) section. This file can be
passed to exporter as a CLI argument as follows:

```bash
ceems_exporter --web.config.file=/path/to/web/config/file
```

The basic auth password is hashed inside the web configuration file just like in
`/etc/passwd` file and hence, the chances of password leaks are minimal.

:::important[IMPORTANT]

In all the cases, it is important that either exporter binary or exporter process must
have enough privileges to be able to export all the metrics. More info on the privileges
necessary for the exporter are discussed in [Configuration](../configuration/ceems-exporter.md)
section where as how to set privileges are briefed in [Security](../configuration/security.md)
section.

:::

Once the exporter is running, by making a request to `/metrics` endpoint will give
following output:

```bash
# HELP ceems_compute_unit_cpu_psi_seconds Total CPU PSI in seconds
# TYPE ceems_compute_unit_cpu_psi_seconds gauge
ceems_compute_unit_cpu_psi_seconds{hostname="myhost",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 0
ceems_compute_unit_cpu_psi_seconds{hostname="myhost",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 0
ceems_compute_unit_cpu_psi_seconds{hostname="myhost",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 0
# HELP ceems_compute_unit_cpu_system_seconds_total Total job CPU system seconds
# TYPE ceems_compute_unit_cpu_system_seconds_total counter
ceems_compute_unit_cpu_system_seconds_total{hostname="myhost",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 115.777502
ceems_compute_unit_cpu_system_seconds_total{hostname="myhost",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 115.777502
ceems_compute_unit_cpu_system_seconds_total{hostname="myhost",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 115.777502
# HELP ceems_compute_unit_cpu_user_seconds_total Total job CPU user seconds
# TYPE ceems_compute_unit_cpu_user_seconds_total counter
ceems_compute_unit_cpu_user_seconds_total{hostname="myhost",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 60375.292848
ceems_compute_unit_cpu_user_seconds_total{hostname="myhost",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 60375.292848
ceems_compute_unit_cpu_user_seconds_total{hostname="myhost",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 60375.292848
# HELP ceems_compute_unit_cpus Total number of job CPUs
# TYPE ceems_compute_unit_cpus gauge
ceems_compute_unit_cpus{hostname="myhost",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 2
ceems_compute_unit_cpus{hostname="myhost",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 2
ceems_compute_unit_cpus{hostname="myhost",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 2
# HELP ceems_compute_unit_gpu_index_flag Indicates running job on GPU, 1=job running
# TYPE ceems_compute_unit_gpu_index_flag gauge
ceems_compute_unit_gpu_index_flag{account="testacc",gpuuuid="20170005280c",hindex="-gpu-3",hostname="myhost",index="3",manager="slurm",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 1
ceems_compute_unit_gpu_index_flag{account="testacc",gpuuuid="20180003050c",hindex="-gpu-2",hostname="myhost",index="2",manager="slurm",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 1
ceems_compute_unit_gpu_index_flag{account="testacc2",gpuuuid="20170000800c",hindex="-gpu-0",hostname="myhost",index="0",manager="slurm",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 1
ceems_compute_unit_gpu_index_flag{account="testacc3",gpuuuid="20170003580c",hindex="-gpu-1",hostname="myhost",index="1",manager="slurm",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 1
# HELP ceems_compute_unit_memory_cache_bytes Memory cache used in bytes
# TYPE ceems_compute_unit_memory_cache_bytes gauge
ceems_compute_unit_memory_cache_bytes{hostname="myhost",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 0
ceems_compute_unit_memory_cache_bytes{hostname="myhost",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 0
ceems_compute_unit_memory_cache_bytes{hostname="myhost",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 0
# HELP ceems_compute_unit_memory_fail_count Memory fail count
# TYPE ceems_compute_unit_memory_fail_count gauge
ceems_compute_unit_memory_fail_count{hostname="myhost",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 0
ceems_compute_unit_memory_fail_count{hostname="myhost",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 0
ceems_compute_unit_memory_fail_count{hostname="myhost",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 0
# HELP ceems_compute_unit_memory_psi_seconds Total memory PSI in seconds
# TYPE ceems_compute_unit_memory_psi_seconds gauge
ceems_compute_unit_memory_psi_seconds{hostname="myhost",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 0
ceems_compute_unit_memory_psi_seconds{hostname="myhost",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 0
ceems_compute_unit_memory_psi_seconds{hostname="myhost",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 0
# HELP ceems_compute_unit_memory_rss_bytes Memory RSS used in bytes
# TYPE ceems_compute_unit_memory_rss_bytes gauge
ceems_compute_unit_memory_rss_bytes{hostname="myhost",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 4.098592768e+09
ceems_compute_unit_memory_rss_bytes{hostname="myhost",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 4.098592768e+09
ceems_compute_unit_memory_rss_bytes{hostname="myhost",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 4.098592768e+09
# HELP ceems_compute_unit_memory_total_bytes Memory total in bytes
# TYPE ceems_compute_unit_memory_total_bytes gauge
ceems_compute_unit_memory_total_bytes{hostname="myhost",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 4.294967296e+09
ceems_compute_unit_memory_total_bytes{hostname="myhost",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 4.294967296e+09
ceems_compute_unit_memory_total_bytes{hostname="myhost",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 4.294967296e+09
# HELP ceems_compute_unit_memory_used_bytes Memory used in bytes
# TYPE ceems_compute_unit_memory_used_bytes gauge
ceems_compute_unit_memory_used_bytes{hostname="myhost",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 4.111491072e+09
ceems_compute_unit_memory_used_bytes{hostname="myhost",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 4.111491072e+09
ceems_compute_unit_memory_used_bytes{hostname="myhost",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 4.111491072e+09
# HELP ceems_compute_unit_memsw_fail_count Swap fail count
# TYPE ceems_compute_unit_memsw_fail_count gauge
ceems_compute_unit_memsw_fail_count{hostname="myhost",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 0
ceems_compute_unit_memsw_fail_count{hostname="myhost",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 0
ceems_compute_unit_memsw_fail_count{hostname="myhost",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 0
# HELP ceems_compute_unit_memsw_total_bytes Swap total in bytes
# TYPE ceems_compute_unit_memsw_total_bytes gauge
ceems_compute_unit_memsw_total_bytes{hostname="myhost",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 1.6042172416e+10
ceems_compute_unit_memsw_total_bytes{hostname="myhost",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 1.6042172416e+10
ceems_compute_unit_memsw_total_bytes{hostname="myhost",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 1.6042172416e+10
# HELP ceems_compute_unit_memsw_used_bytes Swap used in bytes
# TYPE ceems_compute_unit_memsw_used_bytes gauge
ceems_compute_unit_memsw_used_bytes{hostname="myhost",manager="slurm",project="testacc",user="testusr",uuid="0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5"} 0
ceems_compute_unit_memsw_used_bytes{hostname="myhost",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 0
ceems_compute_unit_memsw_used_bytes{hostname="myhost",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 0
# HELP ceems_compute_unit_rdma_hca_handles Current number of RDMA HCA handles
# TYPE ceems_compute_unit_rdma_hca_handles gauge
ceems_compute_unit_rdma_hca_handles{device="hfi1_0",hostname="myhost",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 479
ceems_compute_unit_rdma_hca_handles{device="hfi1_0",hostname="myhost",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 289
ceems_compute_unit_rdma_hca_handles{device="hfi1_1",hostname="myhost",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 1479
ceems_compute_unit_rdma_hca_handles{device="hfi1_2",hostname="myhost",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 2479
# HELP ceems_compute_unit_rdma_hca_objects Current number of RDMA HCA objects
# TYPE ceems_compute_unit_rdma_hca_objects gauge
ceems_compute_unit_rdma_hca_objects{device="hfi1_0",hostname="myhost",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 479
ceems_compute_unit_rdma_hca_objects{device="hfi1_0",hostname="myhost",manager="slurm",project="testacc3",user="testusr2",uuid="77caf800-acd0-1fd2-7211-644e46814fc1"} 289
ceems_compute_unit_rdma_hca_objects{device="hfi1_1",hostname="myhost",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 1479
ceems_compute_unit_rdma_hca_objects{device="hfi1_2",hostname="myhost",manager="slurm",project="testacc2",user="testusr2",uuid="018ce2fe-b3f9-632a-7507-0e01c2687de5"} 2479
# HELP ceems_compute_units Total number of jobs
# TYPE ceems_compute_units gauge
ceems_compute_units{hostname="myhost",manager="slurm"} 3
# HELP ceems_cpu_count Number of CPUs.
# TYPE ceems_cpu_count gauge
ceems_cpu_count{hostname="myhost"} 8
# HELP ceems_cpu_seconds_total Seconds the CPUs spent in each mode.
# TYPE ceems_cpu_seconds_total counter
ceems_cpu_seconds_total{hostname="myhost",mode="idle"} 89790.04
ceems_cpu_seconds_total{hostname="myhost",mode="iowait"} 35.52
ceems_cpu_seconds_total{hostname="myhost",mode="irq"} 0.02
ceems_cpu_seconds_total{hostname="myhost",mode="nice"} 6.12
ceems_cpu_seconds_total{hostname="myhost",mode="softirq"} 39.44
ceems_cpu_seconds_total{hostname="myhost",mode="steal"} 0
ceems_cpu_seconds_total{hostname="myhost",mode="system"} 1119.22
ceems_cpu_seconds_total{hostname="myhost",mode="user"} 3018.54
# HELP ceems_exporter_build_info A metric with a constant '1' value labeled by version, revision, branch, goversion from which ceems_exporter was built, and the goos and goarch for the build.
# TYPE ceems_exporter_build_info gauge
# HELP ceems_ipmi_dcmi_avg_watts Average Power consumption in watts
# TYPE ceems_ipmi_dcmi_avg_watts gauge
ceems_ipmi_dcmi_avg_watts{hostname="myhost"} 5942
# HELP ceems_ipmi_dcmi_current_watts Current Power consumption in watts
# TYPE ceems_ipmi_dcmi_current_watts gauge
ceems_ipmi_dcmi_current_watts{hostname="myhost"} 5942
# HELP ceems_ipmi_dcmi_max_watts Maximum Power consumption in watts
# TYPE ceems_ipmi_dcmi_max_watts gauge
ceems_ipmi_dcmi_max_watts{hostname="myhost"} 6132
# HELP ceems_ipmi_dcmi_min_watts Minimum Power consumption in watts
# TYPE ceems_ipmi_dcmi_min_watts gauge
ceems_ipmi_dcmi_min_watts{hostname="myhost"} 5748
# HELP ceems_meminfo_MemAvailable_bytes Memory information field MemAvailable_bytes.
# TYPE ceems_meminfo_MemAvailable_bytes gauge
ceems_meminfo_MemAvailable_bytes{hostname="myhost"} 0
# HELP ceems_meminfo_MemFree_bytes Memory information field MemFree_bytes.
# TYPE ceems_meminfo_MemFree_bytes gauge
ceems_meminfo_MemFree_bytes{hostname="myhost"} 4.50891776e+08
# HELP ceems_meminfo_MemTotal_bytes Memory information field MemTotal_bytes.
# TYPE ceems_meminfo_MemTotal_bytes gauge
ceems_meminfo_MemTotal_bytes{hostname="myhost"} 1.6042172416e+10
# HELP ceems_rapl_package_joules_total Current RAPL package value in joules
# TYPE ceems_rapl_package_joules_total counter
ceems_rapl_package_joules_total{hostname="myhost",index="0",path="pkg/collector/testdata/sys/class/powercap/intel-rapl:0"} 258218.293244
ceems_rapl_package_joules_total{hostname="myhost",index="1",path="pkg/collector/testdata/sys/class/powercap/intel-rapl:1"} 130570.505826
```
