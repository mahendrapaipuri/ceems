---
sidebar_position: 8
---

# cacct

`cacct` is a CLI client that can be used in the place of Grafana when the
operators cannot/do not wish to maintain a Grafana instance. This CLI client
talks to both CEEMS API server and TSDB server to fetch the energy, usage and
performance metrics of a given compute unit and/or project and/or user. This
has been largely inspired from SLURM's [`sacct`](https://slurm.schedmd.com/sacct.html)
tool and the API resembles that of `sacct`.

:::important[IMPORTANT]

`cacct` identifies the current username from their Linux's UID. Thus, for `cacct`
to work correctly, the user's UID must be the same on the machine where `cacct`
is being executed and in the CEEMS API server DB.

:::

This tool has been specifically designed for HPC platforms where there a common
login node that users can access _via_ SSH. Th tool must be installed on such
login nodes along with its configuration file. The `cacct`'s configuration file
contains the HTTP client configuration details to connect to CEEMS API and
TSB servers. Thus, this configuration file would potentially contains secrets to
talk to these servers and it is very important to protect this file on a multi-tenant
system like HPC login nodes. This will be discussed more in the following sections. First,
let's take a look at the available configuration sections for `cacct`:

```yaml
# cacct configuration skeleton

ceems_api_server: <CEEMS API SERVER CONFIG>

tsdb: <TSDB CONFIG>
```

:::important[IMPORTANT]

`cacct` always looks for the configuration file at `/etc/ceems/config.yml` or
`/etc/ceems/config.yaml`. Thus, configuration file must be installed in one
of these locations.

:::

A sample configuration file with only CEEMS API Server config is presented below:

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

The above configuration assumes that the target cluster has `slurm-0` as cluster ID configured
in the configuration of [CEEMS API server](./ceems-api-server.md#clusters-configuration). By default,
CEEMS API server expects the username in the header `X-Grafana-User` so that `cacct` sets the value for
this header with the username that is making the request. Finally, section `web` contains the HTTP client
configuration of the CEEMS API server. In the above example, CEEMS API server is reachable at host
`ceems-api-server` and on port `9020` and basic auth is configured to the CEEMS API server.

`cacct` is capable of pulling the time series data from TSDB server of the requested compute units and
it is possible to do so only when `tsdb` section has been configured. A sample configuration file
with CEEMS API server and TSDB server configs is:

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
    # CPU utilisation
    cpu_usage: uuid:ceems_cpu_usage:ratio_irate{uuid=~"%s"}
    
    # CPU Memory utilisation
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

    # Read IO bytes
    io_read_bytes: irate(ceems_ebpf_read_bytes_total{uuid=~"%s"}[1m])

    # Write IO bytes
    io_write_bytes: irate(ceems_ebpf_write_bytes_total{uuid=~"%s"}[1m])
```

Just like in the case of CEEMS API server, the above configuration assumes
that TSDB server is reachable at `tsdb:9090` and basic auth has been configured
on the HTTP server. The section `tsdb.queries` is where the operators need
to configure the queries to pull the time series data of each metric. If
the operators have used [`ceems_tool`](../usage/ceems-tool.md) to generate
recording rules for TSDB, the queries used in the above configuration sample
file will work out-of-the-box. The key of `queries` object can be chosen
freely and it is provided for the maintainability of the configuration file.
The placeholder `%s` will be replaced by the compute unit UUIDs at runtime
before executing queries on TSDB server.

:::note[NOTE]

There is no risk of injection here as the UUID values provided by the end-user
are first sanitized and then verified with CEEMS API server to check if the user
is owner of the compute unit before passing them to TSDB server.

:::

A complete reference can be found in [Reference](./config-reference.md)
section. A valid sample configuration
file can be found in the [repo](https://github.com/mahendrapaipuri/ceems/blob/main/build/config/cacct/cacct.yml)

## Securing configuration file

As evident from the previous section, the configuration file of `cacct` will contain
secrets that should be accessible to the end users. At the same time, the executable
`cacct` must be accessible to the end-users to be able to fetch their usage statistics.
This means, `cacct` must be able to read the configuration file at the runtime but not
the user who is executing it. This can be done using [Sticky bit](https://www.redhat.com/en/blog/suid-sgid-sticky-bit).

By using SETUID or SETGID bit on the executable, the binary will execute as the user or
group that owns the file and not the user who invokes the execution. For instance, imagine
a case where a system user/group `ceems` is created on a HPC login node. The sticky bit SETGID
can be set on the `cacct` as follows:

```bash
chown ceems:ceems /usr/local/bin/cacct
chmod g+s /usr/local/bin/cacct
# Ensure others can execute cacct
chmod o+x /usr/local/bin/cacct

# Use the same user/group as owner:group to cacct configuration file
chown ceems:ceems /etc/ceems/config.yml
# Revoke all the permissions for others
chmod o-rwx /etc/ceems/config.yml
```

Now everytime `cacct` has been invoked, it runs as `ceems` user instead of user who invoked
it. As the same user/group is the owner to the file `/etc/ceems/config.yml`, `cacct` will be
able to read the file. At the same time, the user who invoked the `cacct` binary will not be
able to access `/etc/ceems/config.yml` as the permission have been revoked.

When `cacct` has been installed using the RPM/DEB file provided by the
[CEEMS Releases](https://github.com/mahendrapaipuri/ceems/releases), `cacct` will be already
installed with sticky bit and the operators only need to populate configuration file at
`/etc/ceems/config.yml`.
