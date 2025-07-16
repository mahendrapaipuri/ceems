---
sidebar_position: 1
---

# CEEMS Exporter

## Basic Usage

To run the exporter with the default enabled collectors, use the following command:

```bash
ceems_exporter 
```

The following collectors are enabled by default:

- `cpu`: Node-level CPU statistics
- `memory`: Node-level memory statistics
- `rapl`: RAPL energy counters

By default, the CEEMS exporter exposes metrics on all interfaces, port `9010`, and
at the `/metrics` endpoint. This can be changed by setting the `--web.listen-address` CLI flag:

```bash
ceems_exporter --web.listen-address="localhost:8010"
```

The above command will run the exporter only on `localhost` and port `8010`.

:::tip[TIP]

All available command-line options are listed in the
[CEEMS Exporter CLI documentation](../cli/ceems-exporter.md).

:::

To enable the SLURM collector, add the following CLI flag:

```bash
ceems_exporter --collector.slurm
```

:::important[IMPORTANT]

Starting from `v0.3.0`, there is no need to configure the GPU type. The exporter will
automatically detect supported GPU types: NVIDIA and AMD.

:::

To disable default collectors, add the `no` prefix to the collector flag.
The following command will disable the IPMI and RAPL collectors:

```bash
ceems_exporter --no-collector.rapl
```

By default, no authentication is imposed on the exporter web server. In production, this
is not advisable, and it is possible to add basic authentication and TLS to the exporter using a web configuration file. More details on how to set up web configuration are discussed in the [Web Configuration](../configuration/basic-auth.md) section. This file can be passed to the exporter as a CLI argument as follows:

```bash
ceems_exporter --web.config.file=/path/to/web/config/file
```

The basic authentication password is hashed inside the web configuration file, similar to
the `/etc/passwd` file, and therefore, the chances of password leaks are minimal.

:::important[IMPORTANT]

In all cases, it is important that either the exporter binary or the exporter process must
have sufficient privileges to export all metrics. More information about the privileges
required for the exporter is discussed in the [Configuration](../configuration/ceems-exporter.md)
section, while how to set privileges is briefed in the [Security](../configuration/security.md)
section.

:::

Once the exporter is running, making a request to the `/metrics` endpoint will provide
the following output:

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
