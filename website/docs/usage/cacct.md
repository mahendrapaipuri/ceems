---
sidebar_position: 4
---

# cacct

`cacct` is a client tool that can talk to CEEMS API server and TSDB server to return
the usage statistics and metrics of user/project/compute unit. The tool has been
designed to closely resemble with SLURM's [`sacct`](ttps://slurm.schedmd.com/sacct.html) tool.
All the available options can be listed using `--help`

```bash
cacct --help
```

:::tip[TIP]

All the available command line options are listed in
[cacct CLI docs](../cli/cacct.md).

:::

## Basic usage

To get list of compute units and their aggregated metrics between `<start>` and `<end>`
dates, following command can be used

```bash
cacct --starttime=<start> --endtime=<end>
```

where `<start>` and `<end>` can be of format `YYYY-MM-DD` or `YYY-MM-DDTHH:MM`.

It is possible to get individual compute unit(s) metric by using `--job` flag with comma
separated list of compute unit IDs as follows:

```bash
cacct --job=12423,443433
```

If the current user is part of multiple accounts/projects, it is possible to limit accounts
in the output using `--account` flag:

```bash
cacct --account=foo,bar
```

For the users who are listed as admin users in [CEEMS API server](../configuration/ceems-api-server.md#ceems-api-server-configuration),
it is possible to consult the accounting statistics of any users/projects/compute units. The
username declared in the CEEMS API server configuration must match with current Linux username.
For instance, if an admin user wants to consult the accounting data of users `usr1` and `usr2` between
`2025-01-01` to `2025-01-31`, it can be done as follows:

```bash
cacct --user=usr1,usr2 --starttime=2025-01-01 --endtime=2025-01-31
```

:::note[NOTE]

If the current user is not in admin users list and attempt to get accounting statistics
of other users, an empty response will be returned.

:::

## Time series data

Besides aggregate accounting statistics, `cacct` is capable of fetching the time series
data of individual compute units and dump them in CSV format. In order to get time series
data, `--ts` flag must be passed.

:::important[IMPORTANT]

When `--ts` flag is used, it is compulsory to set at least one compute unit ID using
`--job` flag. If the users want time series data of multiple jobs, comma separated list
of IDs can be passed to `--job` flag.

:::

```bash
cacct --job=1234,1233 --ts --ts.out-dir=data
```

With above command, the time series data of the compute units 1234 and 1233 will be saved
in CSV format in `data` directory of current working directory. Inside the `data` folder,
there will be a file `metadata.json` where fingerprint of each series and metadata of the
series will be saved. The CSV files will be named after the fingerprints. For instance,
a typical `metadata.json` would be as follows:

```json
[
 {
  "fingerprint": "d2213312c639a90c",
  "labels": {
   "__name__": "uuid:ceems_host_emissions_g_s:pue",
   "hostname": "ceems-demo",
   "instance": "localhost:9010",
   "job": "slurm",
   "manager": "slurm",
   "provider": "owid",
   "uuid": "258"
  }
 },
 {
  "fingerprint": "85105ad7ffcf540a",
  "labels": {
   "__name__": "uuid:ceems_host_emissions_g_s:pue",
   "hostname": "ceems-demo",
   "instance": "localhost:9010",
   "job": "slurm",
   "manager": "slurm",
   "provider": "rte",
   "uuid": "258"
  }
 },
 {
  "fingerprint": "c819bde6e9a529b6",
  "labels": {
   "__name__": "uuid:ceems_cpu_memory_usage:ratio",
   "hostname": "ceems-demo",
   "instance": "localhost:9010",
   "job": "slurm",
   "manager": "slurm",
   "uuid": "258"
  }
 },
 {
  "fingerprint": "90bcc7cfa3cd05fa",
  "labels": {
   "__name__": "uuid:ceems_cpu_usage:ratio_irate",
   "hostname": "ceems-demo",
   "instance": "localhost:9010",
   "job": "slurm",
   "manager": "slurm",
   "uuid": "258"
  }
 },
 {
  "fingerprint": "cbba6b4919ac1bad",
  "labels": {
   "__name__": "uuid:ceems_host_power_watts:pue",
   "hostname": "ceems-demo",
   "instance": "localhost:9010",
   "job": "slurm",
   "manager": "slurm",
   "uuid": "258"
  }
 }
]
```

And CSV files will be named after fingerprints. For instance a file
`cbba6b4919ac1bad.csv` will have time series data of host power of compute
unit `258`. Similarly, `90bcc7cfa3cd05fa.csv` will have data of CPU usage
of unit `258`.
