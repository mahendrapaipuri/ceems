---
sidebar_position: 2
---

# CEEMS Exporter

Different collectors of CEEMS exporter are briefed earlier in
[Components](../components/ceems-exporter.md) section. Some of these collectors need
privileges to collect metrics. Current list of collectors that need privileges are
listed below.

:::important[IMPORTANT]

Starting from `v0.3.0`, the following CLI flags have been slightly modified to have
a consistent styling. They will be removed in `v1.0.0`.

- `--collector.slurm.swap.memory.metrics` changed to `--collector.slurm.swap-memory-metrics`
- `--collector.slurm.psi.metrics` changed to `--collector.slurm.psi-metrics`
- `--collector.meminfo.all.stats` changed to `--collector.meminfo.all-stats`
- `--collector.ipmi.dcmi.cmd` changed to `--collector.ipmi_dcmi.cmd`

:::

## Slurm collector

Although fetching metrics from cgroups do not need any additional privileges, getting
GPU ordinal to job ID needs extra privileges. This is due to the fact that this
information is not readily available in cgroups. Currently, the exporter supports two different
ways to get the GPU ordinals to job ID map.

- Reading environment variables `SLURM_STEP_GPUS` and/or `SLURM_JOB_GPUS` of job from
`/proc` file system which contains GPU ordinal numbers of job.
- Use prolog and epilog scripts to get the GPU to job ID map. Example prolog script
is provided in the [repo](https://github.com/mahendrapaipuri/ceems/tree/main/etc/slurm).

We recommend to use the first approach as it requires minimum configuration to maintain
for the operators. The downside is that the CEEMS exporter process will need some
privileges to be able to read the environment variables in `/proc` file system. The
privileges can be set in different ways and it is discussed in [Security](./security.md)
section.

On the other hand, if the operators do not wish to add any privileges to exporter
process, they can use the second approach but this requires some configuration additions
to SLURM controller to execute a prolog and epilog script for each job.

A sample prolog script to get GPU ordinals is as follows:

```bash
#!/bin/bash

# Need to use this path in --collector.nvidia.gpu-job-map-path flag for ceems_exporter
DEST=/run/gpujobmap
[ -e $DEST ] || mkdir -m 755 $DEST

# CUDA_VISIBLE_DEVICES in prolog will be "actual" GPU indices and once job starts
# CUDA will reset the indices to always start from 0. Thus inside a job, CUDA_VISIBLE_DEVICES
# will always start with 0 but during prolog script execution it can be any ordinal index
# based on how SLURM allocated the GPUs
# Ref: https://slurm.schedmd.com/prolog_epilog.html
for i in ${GPU_DEVICE_ORDINAL//,/ } ${CUDA_VISIBLE_DEVICES//,/ }; do
  echo $SLURM_JOB_ID > $DEST/$i
done
exit 0 
```

At the end of each job, we must remove these files from `/run` file system to avoid
accumulation of these files. This can be configured using epilog scrips and sample
scripts can be found in the [repo](https://github.com/mahendrapaipuri/ceems/tree/main/etc/slurm/epilog.d).
These prolog and epilog scripts must be configured to run at the start and end of each
job and operators can consult [SLURM docs](https://slurm.schedmd.com/prolog_epilog.html)
on more details configuring epilog and prolog scripts.

Assuming the operators are using the above prolog scripts to get job meta data, CEEMS
exporter must be configured with the following CLI flags:

```bash
ceems_exporter --collector.slum --collector.slurm.gpu-job-map-path=/run/gpujobmap
```

With above configuration, the exporter should export GPU ordinal mapping
along with other metrics of slurm collector.

When compute nodes uses a mix of full physical GPUs and MIG instances (NVIDIA), the
ordering of GPUs by SLURM is undefined and it can depend on how the compute nodes are
configured. More details can be found in this
[bug report](https://support.schedmd.com/show_bug.cgi?id=21163). If ordering of GPUs
does not match between `nvidia-smi` and SLURM, operators need to configure CEEMS
exporter appropriately to provide the information about ordering of GPUs known to SLURM.

For example if there are 2 A100 GPUs on a compute node where MIG is enabled only on
GPU 0.

```bash
$ nvidia-smi
Fri Oct 11 12:04:56 2024       
+---------------------------------------------------------------------------------------+
| NVIDIA-SMI 535.129.03             Driver Version: 535.129.03   CUDA Version: 12.2     |
|-----------------------------------------+----------------------+----------------------+
| GPU  Name                 Persistence-M | Bus-Id        Disp.A | Volatile Uncorr. ECC |
| Fan  Temp   Perf          Pwr:Usage/Cap |         Memory-Usage | GPU-Util  Compute M. |
|                                         |                      |               MIG M. |
|=========================================+======================+======================|
|   0  NVIDIA A100-PCIE-40GB          On  | 00000000:21:00.0 Off |                   On |
| N/A   28C    P0              31W / 250W |     50MiB / 40960MiB |     N/A      Default |
|                                         |                      |              Enabled |
+-----------------------------------------+----------------------+----------------------+
|   1  NVIDIA A100-PCIE-40GB          On  | 00000000:81:00.0 Off |                    0 |
| N/A   27C    P0              34W / 250W |      4MiB / 40960MiB |      0%      Default |
|                                         |                      |             Disabled |
+-----------------------------------------+----------------------+----------------------+

+---------------------------------------------------------------------------------------+
| MIG devices:                                                                          |
+------------------+--------------------------------+-----------+-----------------------+
| GPU  GI  CI  MIG |                   Memory-Usage |        Vol|      Shared           |
|      ID  ID  Dev |                     BAR1-Usage | SM     Unc| CE ENC DEC OFA JPG    |
|                  |                                |        ECC|                       |
|==================+================================+===========+=======================|
|  0    3   0   0  |              12MiB /  9856MiB  | 14      0 |  1   0    1    0    0 |
|                  |               0MiB / 16383MiB  |           |                       |
+------------------+--------------------------------+-----------+-----------------------+
|  0    4   0   1  |              12MiB /  9856MiB  | 14      0 |  1   0    1    0    0 |
|                  |               0MiB / 16383MiB  |           |                       |
+------------------+--------------------------------+-----------+-----------------------+
|  0    5   0   2  |              12MiB /  9856MiB  | 14      0 |  1   0    1    0    0 |
|                  |               0MiB / 16383MiB  |           |                       |
+------------------+--------------------------------+-----------+-----------------------+
|  0    6   0   3  |              12MiB /  9856MiB  | 14      0 |  1   0    1    0    0 |
|                  |               0MiB / 16383MiB  |           |                       |
+------------------+--------------------------------+-----------+-----------------------+
                                                                                         
+---------------------------------------------------------------------------------------+
| Processes:                                                                            |
|  GPU   GI   CI        PID   Type   Process name                            GPU Memory |
|        ID   ID                                                             Usage      |
|=======================================================================================|
|  No running processes found                                                           |
+---------------------------------------------------------------------------------------+
```

In this case `nvidia-smi` orders GPUs and MIG instances as follows:

- 0.3
- 0.4
- 0.5
- 0.6
- 1

where `0.3` indicates GPU 0 and GPU Instance ID (GI ID) 3. However, SLURM can order these
GPUs as follows depending on certain configurations:

- 1
- 0.3
- 0.4
- 0.5
- 0.6

The difference between two orderings is that SLURM is placing full physical GPU at the top
and then enumerating MIG instances. The operators can verify the ordering of SLURM GPUs
by reserving a job and looking at `SLURM_JOB_GPUS` or `SLURM_STEP_GPUS` environment variables.
If the odering is different between `nvidia-smi` and SLURM as demonstrated in this example,
we need to define a map from SLURM order to `nvidia-smi` order and pass it to exporter using
`--collector.slurm.gpu-order-map` CLI flag. In this case, the map definition would be
`--collector.slurm.gpu-order-map=0:1,1:0.3,2:0.4,3:0.5,4:0.6`. The nomenclature is
`<slurm_gpu_index>:<nvidia_gpu_idex>.[<mig_gpu_instance_id>]` delimited by `,`. From
SLURM's point-of-view, GPU 0 is GPU 1 from `nvidia-smi`'s point-of-view and hence first
element is `0:1`. Similarly, SLURM's GPU 1 is `nvidia-smi`'s GPU `0.3` (GPU 0 with GI ID 3)
and hence second element is `1:0.3` and so on. As stated above, if the compute node uses
either full GPUs and if all GPUs are MIG partitioned, the order between SLURM and `nvidia-smi`
would be the same. In any case, it is a good idea to ensure the GPU indexes agree between
SLURM and `nvidia-smi` and configuring CEEMS exporter appropriately.

As discussed in [Components](../components/ceems-exporter.md#slurm-collector), Slurm
collector supports [perf](../components/ceems-exporter.md#perf-sub-collector) and
[eBPF](../components/ceems-exporter.md#ebpf-sub-collector) sub-collectors. These
sub-collectors can be enabled using following CLI flags:

:::warning[WARNING]

eBPF sub-collector needs a kernel version `>= 5.8`.

:::

```bash
ceems_exporter --collector.slurm --collector.perf.hardware-events --collector.perf.software-events --collector.perf.hardware-cache-events --collector.ebpf.io-metrics --collector.ebpf.network-metrics
```

The above command will enable hardware, software and hardware cache perf metrics along
with IO and network metrics retrieved by eBPF sub-collector.

In production, users may not wish to profile their codes _all the time_ even though
the overhead induced by these monitoring these metrics is negligible. In order to
tackle this usecase, collection of perf metrics can be triggered by the presence of
a configured environment variable. Operators need to choose an environment variable(s)
name and configure it with the exporter as follows:

```bash
ceems_exporter --collector.slurm --collector.perf.hardware-events --collector.perf.software-events --collector.perf.hardware-cache-events --collector.perf.env-var=CEEMS_ENABLE_PERF --collector.perf.env-var=ENABLE_PERF
```

The above example command will enable all available perf metrics and monitor the processes
in a SLURM job, _only if one of `CEEMS_ENABLE_PERF` or `ENABLE_PERF` environment variable is set_.

:::note[NOTE]

As demonstrated in the example, more than one environment variable can be configured and
presence of at least one of the configured environment variables is enough to trigger
the perf metrics monitoring.

:::

The presence of environment variable is enough to trigger the monitoring of perf metrics and
the value of the environment variable is not checked. Thus, an environment variable like
`CEEMS_ENABLE_PERF=false` will trigger the perf metrics monitoring. The operators need to
inform their end users to set one of these configured environment variables in their
workflows to have the perf metrics monitored.

:::important[IMPORTANT]

This way of controlling the monitoring of metrics is only applicable to perf events namely,
hardware, software and hardware cache events. Unfortunately there is no easy way to use a
similar approach for IO and network metrics which are provided by eBPF sub-collector. This
is due to the fact that these metrics are collected in the kernel space and ability to
enable and disable them at runtime is more involved.

:::

Both perf and eBPF sub-collectors extra privileges to work and the necessary privileges
are discussed in [Security](./security.md) section.

## Libvirt collector

Libvirt collector is meant to be used on Openstack cluster where VMs are managed by
libvirt. Most of the options applicable to Slurm are applicable to libvirt as well.
For the case of GPU mapping, the exporter will fetch this information directly from
instance's XML file. The exporter can be launched as follows to enable libvirt
collector:

```bash
ceems_exporter --collector.libvirt
```

Both ebpf and perf sub-collectors are supported by libvirt collector and they can
be enabled as follows:

```bash
ceems_exporter --collector.libvirt --collector.perf.hardware-events --collector.perf.software-events --collector.perf.hardware-cache-events --collector.ebpf.io-metrics --collector.ebpf.network-metrics
```

:::note[NOTE]

This is not possible to selectively profile processes inside the guest using
`--collector.perf.env-var` as hypervisor will have no information about the
processes inside the guest.

:::

Both perf and eBPF sub-collectors extra privileges to work and the necessary privileges
are discussed in [Security](./security.md) section.

## IPMI collector

Currently, collector supports FreeIPMI, OpenIMPI, IPMIUtils and Cray's [`capmc`](https://cray-hpe.github.io/docs-csm/en-10/operations/power_management/cray_advanced_platform_monitoring_and_control_capmc/)
framework. If one of these binaries exist on `PATH`, the exporter will automatically
detect it and parse the implementation's output to get power reading values.

:::note[NOTE]

Current auto detection mode is only limited to `ipmi-dcmi` (FreeIPMI), `ipmitool`
(OpenIPMI), `ipmiutil` (IPMIUtils) and `capmc` (Cray) implementations. These binaries
must be on `PATH` for the exporter to detect them. If a custom IPMI command is used,
the command must output the power info in
[one of these formats](https://github.com/mahendrapaipuri/ceems/blob/c031e0e5b484c30ad8b6e2b68e35874441e9d167/pkg/collector/ipmi.go#L35-L92).
If that is not the case, operators must write a wrapper around the custom IPMI command
to output the energy info in one of the supported formats. When a custom script is being
used, it is possible to configure it using CLI flag `--collector.ipmi_dcmi.cmd`.

:::

Generally `ipmi` related commands are available for only `root`. Like in the case of
slurm collector, there are different ways to configure the privileges to execute
IPMI command.

- Admins can add a sudoers entry to let the user that runs the `ceems_exporter` to
execute only necessary command that reports the power usage. For instance, in the case of FreeIPMI
implementation, that sudoers entry will be

```plain
ceems ALL = NOPASSWD: /usr/sbin/ipmi-dcmi
```

The exporter will automatically attempt to run the discovered IPMI command with `sudo`
prefix.

- Use linux capabilities to spawn a subprocess as `root` to execute just the `ipmi-dcmi`
command. This needs `CAP_SETUID` and `CAP_SETGID` capabilities in order to able use `setuid` and
`setgid` syscalls.

- Last approach is to run `ceems_exporter` as root.

We recommend to use either `sudo` or capabilities approach. More on the privileges
can be consulted from [Security](./security.md) section.

:::important[IMPORTANT]

When the compute nodes have GPUs, it is important to verify what IPMI DCMI
power reading report exactly. Depending on the vendor's implementation, it might or
might not include the power consumption of GPUs.

:::

## RAPL collector

For the kernels that are `<5.3`, there is no special configuration to be done. If the
kernel version is `>=5.3`, RAPL metrics are only available for `root`. Three approaches
can be envisioned here:

- Adding capability `CAP_DAC_READ_SEARCH` to the exporter process can give enough
privileges to read the energy counters.
- Another approach is to add a ACL rule on the `/sys/fs/class/powercap`
directory to give read permissions to the user that is running `ceems_exporter`.
- Running `ceems_exporter` as `root` user.

We recommend the capabilities approach as it requires minimum configuration.

## Emissions collector

The only configuration needed for emissions collector is an API token for
[Electricity Maps](https://app.electricitymaps.com/map). For non commercial uses,
a [free tier token](https://www.electricitymaps.com/free-tier-api) can be requested.
This token must be passed using an environment variable `EMAPS_API_TOKEN` in the
systemd service file of the collector.

:::tip[TIP]

This collector is not enabled by default as it is not needed to run on every compute node.
This collector can be run separately on a node that has internet access by disabling
rest of the collectors.

:::
