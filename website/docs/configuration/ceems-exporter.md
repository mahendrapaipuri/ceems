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

## Collectors

The following collectors are supported by Prometheus exporter and they can be configured
from CLI arguments as briefed below:

### Slurm collector

Although fetching metrics from cgroups do not need any additional privileges, getting
GPU ordinal to job ID needs extra privileges. This is due to the fact that this
information is not readily available in cgroups. Currently, the exporter gets this
information by reading environment variables `SLURM_STEP_GPUS` and/or `SLURM_JOB_GPUS`
of job from `/proc` file system which contains GPU ordinal numbers of job. CEEMS exporter
process will need some privileges to be able to read the environment variables in `/proc`
file system. The privileges can be set in different ways and it is discussed in
[Security](./security.md) section.

<!-- On the other hand, if the operators do not wish to add any privileges to exporter
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
along with other metrics of slurm collector. -->

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

### Libvirt collector

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

### IPMI collector

:::important[IMPORTANT]

From version `0.5.0`, this collector is disabled by default and it
must be explicitly enabled using CLI flag `--collector.ipmi_dcmi`

:::

Currently, collector supports FreeIPMI, OpenIMPI, IPMIUtils and Cray's [`capmc`](https://cray-hpe.github.io/docs-csm/en-10/operations/power_management/cray_advanced_platform_monitoring_and_control_capmc/)
framework. If one of these binaries exist on `PATH`, the exporter will automatically
detect it and parse the implementation's output to get power reading values.

:::warning[WARNING]

Starting from `0.5.0`, fetching power reading from BMC using third-party libraries
like FreeIPMI, IPMIUtils has been deprecated. The collector supports a pure Golang
implementation of IPMI protocol using
[OpenIPMI driver interface](https://www.kernel.org/doc/html/v5.9/driver-api/ipmi.html).
Currently, this mode is only used as a fallback when no third-party libraries are found
on the host. However, the pure Go implementation is more performant than calling
third-party libraries in a sub-process and it should be preferred over other methods.
Users can force this mode by passing CLI flag `--collector.ipmi_dcmi.force-native-mode`

:::

Thus in order to enable and force IPMI collector in native mode, following CLI flags
must be passed to exporter

```bash
ceems_exporter --collector.ipmi_dcmi --collector.ipmi_dcmi.force-native-mode
```

Generally `ipmi` related commands are available for only `root`. More on the privileges
can be consulted from [Security](./security.md) section.

:::important[IMPORTANT]

When the compute nodes have GPUs, it is important to verify what IPMI DCMI
power reading report exactly. Depending on the vendor's implementation, it might or
might not include the power consumption of GPUs.

:::

### Redfish collector

Redfish exposes the BMC related telemetry data using a REST API server. Thus, this
collector needs the configuration of API server to be able to talk to it and fetch
power consumption data of different the server. By default this collector is disabled
and hence, it needs to be explicitly enabled using following CLI:

```bash
ceems_exporter --collector.redfish
```

A YAML configuration file containing the Redfish's API server details must be provided
to the CLI flag `--collector.redfish.web-config`. A sample file is shown as follows:

```yaml
---
redfish_web_config:
  # Protocol of Redfish API server. Possible values are http, https
  #
  protocol: https

  # Hostname of the Redfish API server. The hostname can accept
  # a `{hostname}` placeholder which will be replaced by the current hostname
  # at runtime.
  #
  # For instance, if a compute node `compute-0` has BMC hostname setup at
  # `compute-0-bmc`, it is possible to provide hostname in the config as
  # `{hostname}-bmc`. At runtime, the placeholder `{hostname}` will be replaced
  # by actual hostname which is `compute-0` and we gives us BMC hostname 
  # `compute-0-bmc`. This lets the operators to deploy exporter on cluster 
  # of nodes using same config file assuming that BMC credentials are also same 
  # across the cluster.
  #
  # If the hostname is not provided, the collector is capable of discovering
  # BMC IP address by making a raw IPMI request to OpenIPMI linux driver.
  # This is equivalent to running `ipmitool lan print` command which will
  # give us BMC LAN IP. This is possible when Linux IPMI driver has been
  # loaded and exporter process has enough privileges (CAP_DAC_OVERRIDE).
  #
  hostname: compute-0-bmc

  # Port to which Redfish API server binds to.
  #
  port: 443

  # External URL at which all Redfish API servers of the cluster are reachable.
  # Generally BMC network is not reachable from the cluster network and hence,
  # we cannot make requests to Redfish API server directly from the compute nodes.
  # In this case, a reverse proxy can be deployed on a management node where
  # BMC network is reachable and proxy the incoming requests to correct Redfish
  # API server target. The `external_url` must point to the URL of this reverse
  # proxy.
  #
  # CEEMS provide a utility `redfish_proxy` app that can do the job of reverse
  # proxy to Redfish API servers.
  #
  # When `external_url` is provided, collector always makes requests to
  # `external_url`. Even when `external_url` is provided, Redfish's web
  # config like `protocol`, `hostname` and `port` must be provided. 
  # Collector will send these details via headers to `redfish_proxy` so
  # that the proxy in-turn makes requests to correct Redfish target
  #
  external_url: http://redfish-proxy:5000

  # Username that has enough privileges to query for chassis power data.
  #
  username: admin

  # Password corresponding to the username provided above.
  #
  password: supersecret

  # If TLS is enabled on Redfish server or Redfish Proxy server 
  # with self signed certificates, set it to true to skip TLS 
  # certificate verification.
  #
  insecure_skip_verify: true

  # If this is set to `true`, a session token will be request with provided username
  # and password once and all the subsequent requests will use that token for auth.
  # If set to `false`, each request will send the provided username and password to
  # perform basic auth.
  #
  # Always prefer to use session tokens by setting this option to `true` as it avoids
  # sending critical username/password credentials in each request and using sessions
  # is more performant than making requests with username/password
  #
  use_session_token: true
```

:::important[IMPORTANT]

This config file contains sensitive information like BMC credentials and hence,
it is very important to impose strict permissions so that the secrets will not be
leaked

When `use_session_token` is set to `true`, ensure the session timeout is more than the scrape interval
of the Prometheus. Otherwise the session will be invalidated before the next scrape
and thus every scrape creates a new session which is not optimal. As a recommendation,
use a session timeout twice as big as scrape interval to avoid situations described above.
More details in [Redfish Spec](https://redfish.dmtf.org/schemas/DSP0266_1.15.1.html#session-lifetime).

:::

Once a file with above config has been placed and secured, say at `/etc/ceems_exporter/redfish-config.yml`,
the collector can be enabled and configured as follows:

```bash
ceems_exporter --collector.redfish --collector.redfish.web-config=/etc/ceems_exporter/redfish-config.yml
```

This configuration assumes that Redfish API server is reachable from the compute node
which might not be the case normally. This is possible when either an
[in-band Host Interface](https://www.dmtf.org/sites/default/files/standards/documents/DSP0270_1.3.1.pdf)
to Redfish has been configured or VLAN has been setup to reach BMC network from compute node.
If this is not the case, BMC network will be only reachable
from management and/or administration nodes. In this case, we need to deploy a proxy that relays the
requests to Redfish API server through the management/admin node.

CEEMS provides a utility proxy application `redfish_proxy` which is distributed
along with other apps that can be used to proxy _in-band_ requests to Redfish. This
proxy can be deployed on a management node that has access to BMC network and use it
as the `external_url` in the Redfish web config of the collector.

Redfish proxy uses a very simple config file that is optional. A sample configuration
file is shown as below:

```yaml
---
# Conifguration file for redfish_proxy app
redfish_config:
  # This section must provide web configuration of
  # Redfish API server.
  #
  web:
    # If Redfish API servers are running with TLS enabled
    # and using self signed certificates, set `insecure_skip_verify`
    # to `true` to skip TLS certifcate verification
    #
    insecure_skip_verify: false
```

If the Redfish servers are running with TLS using self signed certificates, provide
a config file with `insecure_skip_verify` set to `true`. If that is not the case, config
file can be avoided.

<!-- If there are multiple network interfaces with IP addresses on the compute nodes, it is
**strongly advised to add entry for each IP address**. For instance, if a compute node
has IP addresses `10.100.4.1`, `10.100.4.2` and `10.100.4.3` and Redfish server for this
node is running at `https://172.21.4.1` then config must be as follows:

```yaml
redfish_config:
  web:
    insecure_skip_verify: true

  targets:
    - host_ip_addrs: 
        - 10.100.4.1
        - 10.100.4.2
        - 10.100.4.3
        - 10.100.4.4
      url: https://172.21.4.1
``` -->

Assuming management node is `mgmt-0` and starting `redfish_proxy` on that node with the above
config in a file stored at `/etc/redfish_proxy/config.yml` can be done as follows:

```bash
redfish_proxy --config.file=/etc/redfish_proxy/config.yml --web.listen-address=":5000"
```

This will start the redfish proxy on management node running at `mgmt-0:5000`. Finally,
redfish configuration file for exporter should be set as follows:

```yaml
redfish_web_config:
  # Redfish API server config
  protocol: http
  hostname: '{hostname}-bmc'
  port: 5000
  # Redfish proxy is running on mgmt-0 at port 5000
  external_url: http://mgmt-0:5000
  # Redfish proxy will pass these credentials transparently
  # to the target Redfish API server
  username: admin
  password: supersecret
  insecure_skip_verify: true
  use_session_token: true
```

With the above config, the collector will build the Redfish API URL based on `protocol`,
`hostname` and `port` parameters and send that to `redfish_proxy` using a header. The
proxy will read this header and proxy the request to correct Redfish target and eventually
sends the response back to the collector.

### Cray's PM counters collector

There is no special configuration required for Cray's PM counters collector. It is
disabled by default and it can be enabled using `--collector.cray_pm_counters` CLI
flag to the `ceems_exporter`.

### RAPL collector

For the kernels that are `<5.3`, there is no special configuration to be done. If the
kernel version is `>=5.3`, RAPL metrics are only available for `root`. Three approaches
can be envisioned here:

- Adding capability `CAP_DAC_READ_SEARCH` to the exporter process can give enough
privileges to read the energy counters.
- Another approach is to add a ACL rule on the `/sys/fs/class/powercap`
directory to give read permissions to the user that is running `ceems_exporter`.
- Running `ceems_exporter` as `root` user.

We recommend the capabilities approach as it requires minimum configuration.

### Emissions collector

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

## Grafana Alloy targets discoverer

CEEMS exporter exposes a special endpoint that can be used as
[HTTP discovery component](https://grafana.com/docs/alloy/latest/reference/components/discovery/discovery.http/)
which can provide a list of targets to Pyroscope eBPF component for continuous profiling.

Currently, the discovery component supports **only SLURM resource manager**. There is
no added value to continuously profile a VM instance managed by Libvirt from hypervisor
as we will not be able ease to resolve symbols of guest instance from the hypervisor. By
default the discovery component is disabled and it can be enabled using the following
component:

```bash
ceems_exporter --discoverer.alloy-targets.resource-manager=slurm
```

which will collect targets from SLURM jobs on the current node.

:::tip[TIP]

The discovery component runs at a dedicated endpoint which can be configured
using `--web.targets-path`. Thus, it is possible to run both discovery
components and Prometheus collectors at the same time as follows:

```bash
ceems_exporter --collector.slurm --discoverer.alloy-targets.resource-manager=slurm
```

:::

Similar to `perf` sub-collector, it is possible to configure the discovery component
to discover the targets only when certain environment variable is set in the process. For
example if we use the following CLI arguments to the exporter

```bash
ceems_exporter --discoverer.alloy-targets.resource-manager=slurm --discoverer.alloy-targets.env-var=ENABLE_CONTINUOUS_PROFILING
```

only SLURM jobs that have a environment variable `ENABLE_CONTINUOUS_PROFILING` set
in their jobs will be continuously profiled. Multiple environment variable names can
be passed by repeating the CLI argument `--discoverer.alloy-targets.env-var`. The
presence of environment variable triggers the continuous profiling irrespective of
the value set to it.

Once the discovery component is enabled, Grafana Alloy can be configured to get
the targets from this component using following config:

```river
discovery.http "processes" {
  url = "http://localhost:9010/alloy-targets"
  refresh_interval = "10s"
}

pyroscope.write "staging" {
  endpoint {
    url = "http://pyroscope:4040"
  }
}

pyroscope.ebpf "default" {
  collect_interval = "10s"
  forward_to   = [ pyroscope.write.staging.receiver ]
  targets      = discovery.http.processes.output
}
```

The above configuration makes Grafana Alloy to scrape the discovery component
of the exporter every 10 seconds. The output of the discovery component is passed
to Pyroscope eBPF component which will continuously profile the processes and
collect those profiles every 10 seconds. Finally, Pyroscope eBPF components will
send these profiles to Pyroscope. More details on how to configure authentication
and TLS for various components can be consulted from [Grafana Alloy](https://grafana.com/docs/alloy) and
[Grafana Pyroscope](https://grafana.com/docs/pyroscope/latest/introduction/) docs.
