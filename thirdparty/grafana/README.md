# Grafana

This folder contains Grafana related data files like dashboards.
In the folder [dashbaords](./dashboards/), we can find 3 sub folders:

- [admin](./dashboards/admin) folder contains the dashboards for admin operators
- [slurm](./dashboards/slurm/) folder contains the dashboards for SLURM clusters
- [openstack](./dashboards/openstack/) folder contains the dashboards for Openstack clusters

## Dashboards

- [Cluster Status](./dashboards/admin/cluster-status.json) shows the overall usage and
consumption of the cluster. Useful for operators to monitor the cluster usage for different
aspects.
- [SLURM Job Summary](./dashboards/slurm/slurm-job-summary.json) shows the list of SLURM
jobs for a chosen user and project. This dashboard can be deployed for both end users and
admins by setting appropriate dashboard variable `endpoint`.
    - For admin dashboards, set it to `/admin`. This lets admins to consult the jobs and usage
        statistics of _any_ cluster user.
    - For user dashboards, set it to **empty space**. This limits users to consult only
        their projects and jobs submitted by them.
- [SLURM Single Job Metrics](./dashboards/slurm/slurm-single-job-metrics.json) shows metrics
of single job. This dashboard is not meant to be used directly. The `List of Jobs` table in
[SLURM Job Summary](./dashboards/slurm/slurm-job-summary.json) creates hyperlinks to individual
job metrics for each job and clicking the link in the table will redirect the users to
`Single Job Metrics` dashboard with all the dashboard variables correctly populated with
job metadata.
- Similarly [Openstack VM Summary](./dashboards/openstack/os-vm-summary.json) and
[Openstack Single VM Metrics](./dashboards/openstack/os-single-vm-metrics.json) provide
the same functionality as `SLURM job Summary` and `SLURM Single Job Metrics` dashboards,
respectively, but for Openstack VMs.

## Importing Dashboards

Operators need to import these dashboards using
[Grafana's dashboard import](https://grafana.com/docs/grafana/latest/dashboards/build-dashboards/import-dashboards/).
While importing dashboards, they will prompt for certain inputs to configure the variables of the
dashboards. The variables for each dashboard are explained below:

### [Cluster Status](./dashboards/admin/cluster-status.json)

- `Prometheus datasource`: Choose the datasource corresponding to Prometheus server that is scrapping
CEEMS (and DCGM/AMD SMI) exporters.

### [SLURM Job Summary](./dashboards/slurm/slurm-job-summary.json)

- `CEEMS API Server's datasource`: Choose the infinity datasource corresponding to CEEMS API server.
- `CEEMS Cluster ID`: It must the same cluster ID as the one defined in
[CEEMS API Server's cluster configuration](https://mahendrapaipuri.github.io/ceems/docs/configuration/config-reference/#cluster_config)
- `Endpoint`: As briefed in [Dashboards](#dashboards) section, use `/admin` for creating Admin facing
dashboards to consult usage statistics of _any_ user. For user facing dashboards, use an **empty space**.

### [SLURM Single Job Metrics](./dashboards/slurm/slurm-single-job-metrics.json)

- `Prometheus datasource`: Choose the datasource corresponding to Prometheus server that is scrapping
CEEMS (and DCGM/AMD SMI) exporters. If [CEEMS LB](https://mahendrapaipuri.github.io/ceems/docs/components/ceems-lb)
has been enabled, choose the datasource corresponding to CEEMS LB as it ensures the access control
enforcement.

### [Openstack VM Summary](./dashboards/openstack/os-vm-summary.json)

- `CEEMS API Server's datasource`: Choose the infinity datasource corresponding to CEEMS API server.
- `CEEMS Cluster ID`: It must the same cluster ID as the one defined in
[CEEMS API Server's cluster configuration](https://mahendrapaipuri.github.io/ceems/docs/configuration/config-reference/#cluster_config)
- `Endpoint`: As briefed in [Dashboards](#dashboards) section, use `/admin` for creating Admin facing
dashboards to consult usage statistics of _any_ user. For user facing dashboards, use an **empty space**.

### [Openstack Single VM Metrics](./dashboards/openstack/os-single-vm-metrics.json)

- `Prometheus datasource`: Choose the datasource corresponding to Prometheus server that is scrapping
CEEMS (and DCGM/AMD SMI) exporters. If [CEEMS LB](https://mahendrapaipuri.github.io/ceems/docs/components/ceems-lb)
has been enabled, choose the datasource corresponding to CEEMS LB as it ensures the access control
enforcement.
