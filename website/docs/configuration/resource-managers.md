---
sidebar_position: 5
---

# Resource Managers

This section contains information on the configuration required by the supported
resource managers by CEEMS.

## SLURM

[Slurm collector](../components/ceems-exporter.md#slurm-collector) in the CEEMS exporter
relies on the job accounting information (like CPU time and memory usage) in the
cgroups that SLURM create for each job to estimate the energy and emissions for a
given job. However, depending on the cgroups version and SLURM configuration, this
accounting information might not be available. The following section will give guidelines
on how to configure SLURM to ensure that this accounting information is always available.

Starting from [SLURM 22.05](https://slurm.schedmd.com/archive/slurm-22.05.0/cgroups.html)
SLURM supports both cgroups v1 and v2. When cgroups v1 is being used, SLURM might not
contain accounting information in the cgroups.

### cgroups v1

The following configuration will enable necessary cgroups controllers and provide the
accounting information of the jobs when cgroups v1 is used.

As stated in the [cgroups docs of SLURM](https://slurm.schedmd.com/cgroup.conf.html),
cgroups plugin can be controlled by the configuration in this file. An
[example config](https://slurm.schedmd.com/cgroup.conf.html#OPT_/etc/slurm/cgroup.conf)
is also provided which should be a good starting point.

Along the `cgroups.conf` file, certain configuration parameters are required in the
`slurm.conf` file as well. This is provided in the
[SLURM docs](https://slurm.schedmd.com/cgroup.conf.html#OPT_/etc/slurm/slurm.conf) as
well.

:::important[IMPORTANT]

Although `JobAcctGatherType=jobacct_gather/cgroup` is presented as _optional_
configuration parameter, it _must_ be used to get the accounting information on CPU
usage. Without this configuration parameter, CPU time of the job will not be available
in the job's cgroups

:::

Besides the above configuration, [SelectTypeParameters](https://slurm.schedmd.com/slurm.conf.html#OPT_SelectTypeParameters)
must be configured to set core or CPU and memory as consumable resources. This is
highlighted in the documentation of [ConstrainRAMSpace](https://slurm.schedmd.com/cgroup.conf.html#OPT_ConstrainRAMSpace)
configuration parameter in [`cgroups.conf` docs](https://slurm.schedmd.com/cgroup.conf.html).

In conclusion, here are the excerpts of configuration needed:

```ini

# cgroups.conf

ConstrainCores=yes
ConstrainDevices=yes
ConstrainRAMSpace=yes
ConstrainSwapSpace=yes

```

```ini
# slurm.conf

ProctrackType=proctrack/cgroup
TaskPlugin=task/cgroup,task/affinity
JobAcctGatherType=jobacct_gather/cgroup 
SelectType=select/con_tres
SelectTypeParameters=CR_CPU_Memory # or CR_Core_Memory
```

### cgroups v2

In the case of cgroups v2, SLURM should create a proper cgroup for every job without any
special configuration. However, the configuration presented for [cgroups v1](#cgroups-v1)
is applicable for cgroups v2 and it is advised to use that configuration for cgroups v2
as well.

## Libvirt

The libvirt collector is meant to be used for Openstack clusters. There is no special
configuration needed as Openstack will take care of confirguring libvirt and QEMU to
enable all relevant cgroup controllers.
