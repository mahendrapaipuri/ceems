---
sidebar_position: 1
---

# Guide

This section presents a guide to the operators to deploy CEEMS stack. There are two
principal components in the CEEMS stack:

- [CEEMS Exporter](../components/ceems-exporter.md) that must be installed on all
the compute nodes.
- [CEEMS API Server](../components/ceems-api-server.md) that must be installed on a
service node.

Optionally, a third component [CEEMS LB](../components/ceems-lb.md) can be installed on the
same service node as CEEMS API server to enforce access control on metrics.

## Prerequisites

### Compute nodes

There are no special requirements for CEEMS exporter to work on compute nodes. Although the
exporter is not extensively tested on different OS distros/architectures, it should work on
all the major distros supported by SLURM/Openstack. The exporter is very light and when all the
[available collectors](../configuration/ceems-exporter.md#collectors) are
enabled on the exporter, it will have a maximum consumption of memory around 150 MB and takes
a CPU time of around 0.05 seconds per scrape request.

If the compute nodes have NVIDIA GPUs, [NVIDIA DCGM](https://docs.nvidia.com/datacenter/dcgm/latest/index.html)
and [NVIDIA DCGM Exporter](https://github.com/NVIDIA/dcgm-exporter/)
must be installed on the compute nodes. Installation instructions of those packages
can be found in their corresponding docs.

Similarly, if the compute nodes have AMD GPUs, [AMD SMI Exporter](https://www.amd.com/en/developer/e-sms/amdsmi-library.html)
must be installed on the compute nodes to get power consumption and performance
metrics of GPUs.

Finally, for the SLURM clusters, if the continuous profiling of the jobs is required,
[Grafana Alloy](https://grafana.com/docs/alloy/latest/) must be installed on the compute nodes.

### Service node

Different services must be deployed for the CEEMS. They can all be deployed on the
same service node or different nodes. Installing them on a same machine will help
to manage the services easily and reduce the attack surface as all services can be
bound to localhost. The list of required services are:

- Prometheus (compulsory): To scrape metrics from exporters running on compute nodes
- CEEMS API server (compulsory): To store the jobs/VMs data in a standardized DB
- Grafana (compulsory): To construct dashboards to expose metrics for operators and endusers.
- CEEMS LB (optional): To enforce access control to the Prometheus metrics
- Pyroscope (optional): When continuous profiling of SLURM jobs is needed

:::note[NOTE]

The present guide assumes that Prometheus, Pyroscope (if needed) and Grafana are
already installed and configured on the service node. Installation instructions
of each component can be consulted from their respective documentation and hence,
they are omitted here.

:::

Installation instructions for [Prometheus](https://prometheus.io/docs/prometheus/latest/installation/),
[Grafana](https://grafana.com/docs/grafana/latest/setup-grafana/installation/)
and [Pyroscope](https://grafana.com/docs/pyroscope/latest/get-started/) can be found in
their docs. CEEMS API server and CEEMS LB requires very modest system resources
and hence, they can be run alongside Prometheus and Pyroscope on the same service node.
The scaling of this service node must take into account the size of cluster, number
of Prometheus targets, Prometheus data retention period, _etc_. A good recommendation
is to have at least 32 GiB of memory and 8 CPUs which should be enough to host all
the necessary services.

When it comes to the storage, Prometheus works best on local disk storage. Thus,
depending on the required retention period, local SSD/NVMe disks with RAID to achieve
fault tolerance can be a good starting point. There are other options like
[Thanos](https://thanos.io/) and [Cortex](https://cortexmetrics.io/) to
achieve long term storage and fault tolerance for Prometheus data.

## Installation Steps

The installation steps in this section make following assumptions:

- There are two sets of compute nodes: 1 Compute node without GPUs `compute-0` and
1 Compute node with NVIDIA GPUs `compute-gpu-0`.
- A single service node `service-0` is used to install all CEEMS related services
- A management node `mgmt-0` that acts as a admin node for the cluster

All the operations will be done from management node using `ssh` to the remote compute
nodes. For containerized deployments, `podman` will be used along
with [Quadlet](https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html)
to manage container services.

### Preparing Management node

Firstly, all the necessary repositories must be added to the local YUM or DEB repositories.
If local repositories are not maintained, it is possible to download the package files and
install from the package files. The following packages and/or repositories must be added:

- CEEMS Exporter, API Server and Load Balancer RPM and DEB files can be
downloaded from [GH Releases](https://github.com/mahendrapaipuri/ceems/releases/latest).
- When NVIDIA GPUs are present on the cluster [CUDA Repos](https://developer.download.nvidia.com/compute/cuda/repos/)
must be added.
- When continuous profiling is needed, [Grafana Alloy](https://grafana.com/docs/alloy/latest/set-up/install/linux/)
packages must be added and enabled.

### Installing Exporter(s)

Once all the necessary packages are downloaded and/or added to the repositories, they
can be installed on the compute nodes.

On the compute nodes, following packages must be installed:

<details>
  <summary>RHEL/CentOS/Rockylinux/Alma</summary>

  ```bash
    whoami
    # root

    hostname
    # compute-0

    dnf install ceems_exporter -y
  ```

  When nodes have NVIDIA GPUs, we need to install NVIDIA DCGM and NVIDIA DCGM exporter.

  :::note[NOTE]

  Current guide assumes that NVIDIA driver `>=550` and CUDA `>=12` is available on
  compute nodes

  :::

  ```bash
    whoami
    # root

    hostname
    # compute-gpu-0

    dnf install datacenter-gpu-manager-4-core datacenter-gpu-manager-4-cuda12 datacenter-gpu-manager-4-devel datacenter-gpu-manager-4-proprietary datacenter-gpu-manager-4-proprietary-cuda12 datacenter-gpu-manager-exporter -y
  ```

  Optionally, for continuously profiling of SLURM jobs, `alloy` must be installed

  ```bash
    whoami
    # root

    hostname
    # compute-0 or compute-gpu-0

    dnf install alloy -y
  ```
</details>
<details>
  <summary>Debian/Ubuntu</summary>

  ```bash
    whoami
    # root

    hostname
    # compute-0

    apt-get install ceems_exporter -y
  ```

  When nodes have NVIDIA GPUs, we need to install NVIDIA DCGM and NVIDIA DCGM exporter.

  :::note[NOTE]

  Current guide assumes that NVIDIA driver `>=550` and CUDA `>=12` is available on
  compute nodes

  :::

  ```bash
    whoami
    # root

    hostname
    # compute-gpu-0

    apt-get install datacenter-gpu-manager-4-core datacenter-gpu-manager-4-cuda12 datacenter-gpu-manager-4-devel datacenter-gpu-manager-4-proprietary datacenter-gpu-manager-4-proprietary-cuda12 datacenter-gpu-manager-exporter -y
  ```

  Optionally, for continuously profiling of SLURM jobs, `alloy` must be installed

  ```bash
    whoami
    # root

    hostname
    # compute-0 or compute-gpu-0

    apt-get install alloy -y
  ```
</details>

### Configuring Exporter(s)

#### CEEMS Exporter

At minimum, CEEMS exporter must be configured with the CLI arguments that enable the relevant collectors.
This can be done using environment variables which can be provided to systemd service file installed
by the package. For instance, to enable SLURM collector and to disable collector metrics, following must
be added to systemd service file

```bash
whoami
# root

hostname
# compute-0 or compute-gpu-0

cat > /etc/systemd/system/ceems_exporter.service.d/override.conf << EOF
[Service]
Environment=CEEMS_EXPORTER_OPTIONS="--collector.slurm --web.disable-exporter-metrics"
EOF
```

More details on runtime configuration of CEEMS exporter can be consulted from the
[docs](../configuration/ceems-exporter.md).

By default, no authentication is enabled on CEEMS exporter and it is **strongly recommended** to add
at least the basic authentication. This is done using a web configuration file which is installed by
packages. More details on all the available options for web configuration can be found in its
[dedicated section](../configuration/basic-auth.md). Once a basic auth username and password have been
chosen, the `basic_auth_users` section in the web configuration file at `/etc/ceems_exporter/web-config.yml`
must be updated.

Finally, CEEMS exporter must be enabled to start at boot and restarted for the changes to take effect.

```bash
whoami
# root

hostname
# compute-0 or compute-gpu-0

systemctl enable ceems_exporter.service
systemctl restart ceems_exporter.service
```

#### DCGM Exporter

```bash
whoami
# root

hostname
# compute-0 or compute-gpu-0

systemctl enable ceems_exporter.service
systemctl restart ceems_exporter.service
```



Once `go` is installed, clone DCGM exporter repo and build `dcgm-exporter` binary. Always checkout
the latest release tag.

```bash
whoami
# root

mkdir -p /root/ceems && cd /root/ceems
git clone https://github.com/NVIDIA/dcgm-exporter.git
cd dcgm-exporter
git checkout $(git describe --tags --abbrev=0)
make binary
```

This will create a compiled binary at `/root/ceems/dcgm-exporter/cmd/dcgm-exporter/dcgm-exporter`.

### Preparing Compute nodes

For compute nodes with NVIDIA GPUs, we need to install NVIDIA DCGM and NVIDIA DCGM exporter. The
major versions of NVIDIA DCGM and DCGM exporter must match for comptability reasons. Thus, if
the DCGM exporter built in the previous step is of version `3.x`, we need to install DCGM 3.

```bash
whoami
# root

hostname
# mgmt-0

ssh compute-gpu-0


```