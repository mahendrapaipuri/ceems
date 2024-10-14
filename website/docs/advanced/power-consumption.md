---
sidebar_position: 1
---

# Energy consumption estimation

## CPU

TODO

## GPU

CEEMS leverages [dcgm-exporter](https://github.com/NVIDIA/dcgm-exporter) and
[amd-smi-exporter](https://github.com/amd/amd_smi_exporter) to get power
consumption of GPUs. When the resource manager uses full physical GPU, estimating
the power consumption of each compute unit is straight-forward as CEEMS exporter
already exports a metric that maps the compute unit ID to GPU ordinal. However,
NVIDIA GPUs support sharing of one physical GPU amongst different compute units
using Multi Instance GPU (MIG) and GRID vGPU strategies. Currently, `dcgm-exporter`
does not estimate power consumption of each MIG instance or vGPU. Thus, CEEMS uses
following approximation to estimate power consumption of shared GPU instances.

### MIG

CEEMS exporter uses an approximation based on number of
Streaming Multiprocessors (SM) for each
MIG instance profile. For instance, in a typical A100 40GB card, a full GPU can be
split into following profiles:

```bash
$ nvidia-smi mig -lgi
+----------------------------------------------------+
| GPU instances:                                     |
| GPU   Name          Profile  Instance   Placement  |
|                       ID       ID       Start:Size |
|====================================================|
|   0  MIG 1g.5gb       19       13          6:1     |
+----------------------------------------------------+
|   0  MIG 2g.10gb      14        5          4:2     |
+----------------------------------------------------+
|   0  MIG 4g.20gb       5        1          0:4     |
+----------------------------------------------------+
```

It means MIG instance `4g.20gb` has 4/7 of SMs, `2g.10gb` has 2/7 of SMs and `1g.5gb` has
1/7 of SMs. Consequently, the power consumed by entire GPU is divided amongst the different
MIG instances in the ratio of their SMs respectively. For example, if the physical GPU's
power consumption is 140 W, the power consumption of each MIG profile will be estimated as
follows:

- `1g.5gb`: 140 * (1/7) = 20 W
- `2g.10gb`: 140 * (2/7) = 40 W
- `4g.20gb`: 140 * (4/7) = 40 W

The exporter will export the coefficient for each MIG instance which can be used along with
power consumption metric of `dcgm-exporter` to estimate power consumption of individual MIG
instances.

### vGPU

In the case of Libvirt, besides MIG it supports GRID vGPU time sharing. Following scenarios
are possible when GPUs are present on the compute node:

- PCI pass through of NVIDIA and AMD GPUs to the guest VMs
- Virtualization of full GPUs using NVIDIA Grid vGPU
- Virtualization of MIG GPUs using NVIDIA Grid vGPU

If a GPU is added to VM using PCI pass through, this GPU will not be available
for the hypervisor and hence, it cannot be queried or monitored. This is due to
the fact that the GPU will be unbound from the hypervisor and bound to guest.
Thus, energy consumption and GPU metrics for GPUs using PCI passthrough
**will only be available in the guest**.

NVIDIA's vGPU uses mediated devices to expose GPUs in the guest and thus,
GPUs can be queried and monitored from both hypervisor and guest. However,
CEEMS rely on [dcgm-exporter](https://github.com/NVIDIA/dcgm-exporter) to
export GPU energy consumption and usage metrics and it does not support
usage and energy consumption metrics for vGPUs. Thus, CEEMS exporter uses
the following approximation method to estimate energy consumption of each
vGPU which in-turn gives energy consumption of each guest VM.

NVIDIA Grid vGPU time slicing divides the GPU resources equally among all the
active vGPUs at any given time and schedule the work on the given physical
GPU. Thus, if there are 4 vGPUs active on a given physical GPU, each vGPU
will get 25% of the full GPU compute power. Thus, a reasonable approximation
would be to split the current physical GPU power consumption equally among all
vGPUs. The same applies when using vGPU on the top of MIG partition. MIG
already divides the physical GPU into different profiles by assigning a given
number of SMs for each profile as discussed in SLURM collector above. When
multiple vGPUs are running on the top of MIG instance, this coefficient is
further divided by number of active vGPUs. For instance, if there are 4 vGPUs
scheduled on a MIG profile `4g.20gb` where the physical GPU is consuming
140 W, the power consumption of each vGPU would be 140*(4/7)*(1/4) = 20 W.
