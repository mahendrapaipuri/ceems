---
sidebar_position: 2
---

# CEEMS Exporter

Different collectors of CEEMS exporter are briefed earlier in 
[Components](../components/ceems-exporter.md) section. Some of these collectors need 
privileges to collect metrics. Current list of collectors that need privileges are 
listed below.

## Slurm collector

Although fetching metrics from cgroups do not need any additional privileges, getting 
GPU ordinal to job ID needs extra privileges. This is due to the fact that this 
information is not readily available in cgroups (at least in v2 where devices are 
bound to cgroups using BPF programs). Currently, the exporter supports two different 
ways to get the GPU ordinals to job ID map. 

- Reading environment variables `SLURM_STEP_GPUS` and/or `SLURM_JOB_GPUS` of job from 
`/proc` file system which contains GPU ordinal numbers of job.
- Use prolog and epilog scripts to get the GPU to job ID map. Example prolog script 
is provided in the [repo](https://github.com/mahendrapaipuri/ceems/tree/main/etc/slurm).

We recommend to use the first approach as it requires minimum configuration to maintain
for the operators. The downside is that the CEEMS exporter process will need some 
privileges to be able to read the environment variables in `/proc` file system. The 
privileges can be set in different ways and it is discussed in [Systemd](./systemd.md) 
section.

On the other hand, if the operators do not wish to add any privileges to exporter 
process, they can use the second approach but this requires some configuration additions 
to SLURM controller to execute a prolog and epilog script for each job. Alongside GPU 
ordinals to job ID map, the exporter retrieves some other job metadata like job owner,
group, account, _etc_ to facilitate easy querying. These meta data are also gathered 
from prolog scripts.

A sample prolog script to get job meta data is as follows:


```bash
#!/bin/bash

# Need to use this path in --collector.slurm.job.props.path flag for ceems_exporter
DEST=/run/slurmjobprops
[ -e $DEST ] || mkdir -m 755 $DEST

# Important to keep the order as SLURM_JOB_USER SLURM_JOB_ACCOUNT SLURM_JOB_NODELIST
echo $SLURM_JOB_USER $SLURM_JOB_ACCOUNT $SLURM_JOB_NODELIST > $DEST/$SLURM_JOB_ID
exit 0 
```

Similarly, sample prolog script to get GPU ordinals is as follows:

```bash
#!/bin/bash

# Need to use this path in --collector.nvidia.gpu.job.map.path flag for ceems_exporter
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
ceems_exporter --collector.slum --collector.slurm.job.props.path=/run/slurmjobprops --collector.slurm.gpu.job.map.path=/run/gpujobmap
```
With above configuration, the exporter should export job meta data and GPU ordinal mapping 
along with other metrics of slurm collector.

:::important[IMPORTANT]

The CLI arguments `--collector.slurm.job.props.path` and `--collector.slurm.gpu.job.map.path` 
are hidden and cannot be seen in `ceems_exporter --help` output. However, these arguments 
exists in the exporter and can be used.

:::

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
used, it is possible to configure it using CLI flag `--collector.ipmi.dcmi.cmd`.

:::

Generally `ipmi` related commands are available for only `root`. Like in the case of 
slurm collector, there are different ways to configure the privileges to execute 
IPMI command. 

- Admins can add a sudoers entry to let the user that runs the `ceems_exporter` to 
execute only necessary command that reports the power usage. For instance, in the case of FreeIPMI 
implementation, that sudoers entry will be

```
ceems ALL = NOPASSWD: /usr/sbin/ipmi-dcmi
```

The exporter will automatically attempt to run the discovered IPMI command with `sudo` 
prefix.

- Use linux capabilities to spawn a subprocess as `root` to execute just the `ipmi-dcmi` 
command. This needs `CAP_SETUID` and `CAP_SETGID` capabilities in order to able use `setuid` and
`setgid` syscalls.

- Last approach is to run `ceems_exporter` as root.

We recommend to use either `sudo` or capabilities approach. More on the privileges 
can be consulted from [Systemd](./systemd.md) section.

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
