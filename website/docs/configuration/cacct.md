---
sidebar_position: 8
---

# cacct

`cacct` is a CLI client that can be used instead of Grafana when operators cannot or do not wish to maintain a Grafana instance. This CLI client communicates with both the CEEMS API server and the TSDB server to fetch energy, usage, and performance metrics for a given compute unit, project, and/or user. It has been largely inspired by SLURM's [`sacct`](https://slurm.schedmd.com/sacct.html) tool, and the API resembles that of `sacct`.

:::important[IMPORTANT]

`cacct` identifies the current username from their Linux UID. Thus, for `cacct` to work correctly, the user's UID must be the same on the machine where `cacct` is executed and in the CEEMS API server database.

:::

This tool has been specifically designed for HPC platforms where there is a common login node that users can access via SSH. The tool must be installed on such login nodes along with its configuration file. The `cacct` configuration file contains the HTTP client configuration details needed to connect to the CEEMS API and TSDB servers. Consequently, this configuration file might contain secrets for communicating with these servers, making it crucial to protect this file on a multi-tenant system like HPC login nodes. This will be discussed further in the following sections. First, let's examine the available configuration sections for `cacct`:

```yaml
# cacct configuration skeleton

ceems_api_server: <CEEMS API SERVER CONFIG>

tsdb: <TSDB CONFIG>
```

:::important[IMPORTANT]

`cacct` always looks for its configuration file at `/etc/ceems/config.yml` or `/etc/ceems/config.yaml`. Therefore, the configuration file must be installed in one of these locations.

:::

A sample configuration file with only the CEEMS API Server configuration is presented below:

```yaml
ceems_api_server:
  cluster_id: slurm-0
  user_header_name: X-Grafana-User
  web:
    url: http://ceems-api-server:9020
    basic_auth:
      username: ceems
      password: supersecretpassword
```

The above configuration assumes that the target cluster has `slurm-0` as its cluster ID, as configured in the [CEEMS API server configuration](./ceems-api-server.md#clusters-configuration). By default, the CEEMS API server expects the username in the `X-Grafana-User` header, so `cacct` sets the value for this header with the username making the request. Finally, the `web` section contains the HTTP client configuration for the CEEMS API server. In this example, the CEEMS API server is reachable at host `ceems-api-server` on port `9020`, and basic authentication is configured.

`cacct` can pull time series data from the TSDB server for the requested compute units. This is possible only when the `tsdb` section is configured. A sample configuration file including both CEEMS API server and TSDB server configurations is shown below:

```yaml
ceems_api_server:
  cluster_id: slurm-0
  user_header_name: X-Grafana-User
  web:
    url: http://ceems-api-server:9020
    basic_auth:
      username: ceems
      password: supersecretpassword

tsdb:
  web:
    url: http://tsdb:9090
    basic_auth:
      username: prometheus
      password: anothersupersecretpassword
  queries:
    # CPU utilization
    cpu_usage: uuid:ceems_cpu_usage:ratio_irate{uuid=~"%s"}
    
    # CPU Memory utilization
    cpu_mem_usage: uuid:ceems_cpu_memory_usage:ratio{uuid=~"%s"}
      
    # Host power usage in Watts
    host_power_usage: uuid:ceems_host_power_watts:pue{uuid=~"%s"}

    # Host emissions in g/s
    host_emissions: uuid:ceems_host_emissions_g_s:pue{uuid=~"%s"}

    # GPU utilization
    avg_gpu_usage: uuid:ceems_gpu_usage:ratio{uuid=~"%s"}

    # GPU memory utilization
    avg_gpu_mem_usage: uuid:ceems_gpu_memory_usage:ratio{uuid=~"%s"}

    # GPU power usage in Watts
    gpu_power_usage: uuid:ceems_gpu_power_watts:pue{uuid=~"%s"}

    # GPU emissions in g/s
    gpu_emissions: uuid:ceems_gpu_emissions_g_s:pue{uuid=~"%s"}

    # Read IO bytes/s
    io_read_bytes: irate(ceems_ebpf_read_bytes_total{uuid=~"%s"}[1m])

    # Write IO bytes/s
    io_write_bytes: irate(ceems_ebpf_write_bytes_total{uuid=~"%s"}[1m])
```

Similar to the CEEMS API server configuration, this example assumes the TSDB server is reachable at `tsdb:9090` and basic authentication is configured on the HTTP server. The `tsdb.queries` section is where operators configure the queries to pull time series data for each metric. If operators used [`ceems_tool`](../usage/ceems-tool.md) to generate recording rules for the TSDB, the queries in the sample configuration above will work out-of-the-box. The keys in the `queries` object can be chosen freely; they are provided for configuration file maintainability. The placeholder `%s` will be replaced by the compute unit UUIDs at runtime before executing the queries on the TSDB server.

:::note[NOTE]

There is no risk of injection here, as the UUID values provided by the end-user are first sanitized and then verified with the CEEMS API server to check if the user is the owner of the compute unit before passing them to the TSDB server.

:::

A complete reference can be found in the [Reference](./config-reference.md) section. A valid sample configuration file can be found in the [repository](https://github.com/mahendrapaipuri/ceems/blob/main/build/config/cacct/cacct.yml).

## Securing configuration file

As evident from the previous section, the `cacct` configuration file contains secrets that should not be accessible to end-users. At the same time, the `cacct` executable must be accessible to end-users so they can fetch their usage statistics. This means `cacct` must be able to read the configuration file at runtime, but the user executing it should not. This can be achieved using the [Sticky bit](https://www.redhat.com/en/blog/suid-sgid-sticky-bit).

By using the SETUID or SETGID bit on the executable, the binary executes as the user or group that owns the file, not the user who invokes the execution. For instance, imagine a case where a system user/group `ceems` is created on an HPC login node. The SETGID sticky bit can be set on `cacct` as follows:

```bash
chown ceems:ceems /usr/local/bin/cacct
chmod g+s /usr/local/bin/cacct
# Ensure others can execute cacct
chmod o+x /usr/local/bin/cacct

# Use the same user/group as owner:group for the cacct configuration file
chown ceems:ceems /etc/ceems/config.yml
# Revoke all permissions for others
chmod o-rwx /etc/ceems/config.yml
```

Now, every time `cacct` is invoked, it runs as the `ceems` user/group instead of the user who invoked it. Since the same user/group owns `/etc/ceems/config.yml`, `cacct` can read the file. Simultaneously, the user who invoked the `cacct` binary cannot access `/etc/ceems/config.yml` because their permissions have been revoked.

When `cacct` is installed using the RPM/DEB file provided by the [CEEMS Releases](https://github.com/mahendrapaipuri/ceems/releases), `cacct` is already installed with the sticky bit set. Operators only need to populate the configuration file at `/etc/ceems/config.yml`.
