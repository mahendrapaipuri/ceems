# Compute Energy & Emissions Monitoring Stack (CEEMS)
<!-- markdown-link-check-disable -->

|         |                                                                                                                                                                                                                                                                                                                                                                                                                 |
| ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| CI/CD   | [![ci](https://github.com/mahendrapaipuri/ceems/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/mahendrapaipuri/ceems/actions/workflows/ci.yml?query=branch%3Amain) [![CircleCI](https://dl.circleci.com/status-badge/img/circleci/8jSYT1wyKY8mKQRTqNLThX/TzM1Mr3AEAqmehnoCde19R/tree/main.svg?style=svg&circle-token=28db7268f3492790127da28e62e76b0991d59c8b)](https://dl.circleci.com/status-badge/redirect/circleci/8jSYT1wyKY8mKQRTqNLThX/TzM1Mr3AEAqmehnoCde19R/tree/main)  [![Coverage](https://img.shields.io/badge/Coverage-73.9%25-brightgreen)](https://github.com/mahendrapaipuri/ceems/actions/workflows/ci.yml?query=branch%3Amain)                                                                                          |
| Docs    | [![docs](https://img.shields.io/badge/docs-passing-green?style=flat&link=https://mahendrapaipuri.github.io/ceems/docs/)](https://mahendrapaipuri.github.io/ceems/)                                                                                                                                                                                                                               |
| Package | [![Release](https://img.shields.io/github/v/release/mahendrapaipuri/ceems.svg?include_prereleases)](https://github.com/mahendrapaipuri/ceems/releases/latest)                                                                                                                                                                     |
| Meta    | [![GitHub License](https://img.shields.io/github/license/mahendrapaipuri/ceems)](https://github.com/mahendrapaipuri/ceems) [![Go Report Card](https://goreportcard.com/badge/github.com/mahendrapaipuri/ceems)](https://goreportcard.com/report/github.com/mahendrapaipuri/ceems) [![code style](https://img.shields.io/badge/code%20style-gofmt-blue.svg)](https://pkg.go.dev/cmd/gofmt) |

<!-- markdown-link-check-enable -->

<p align="center">
  <img src="https://raw.githubusercontent.com/mahendrapaipuri/ceems/main/website/static/img/logo.png" width="200">
</p>

Compute Energy & Emissions Monitoring Stack (CEEMS) (pronounced as *kiːms*) contains
a Prometheus exporter to export metrics of compute instance units and a REST API
server that serves the metadata and aggregated metrics of each
compute unit. Optionally, it includes a TSDB load balancer that supports basic access
control on TSDB so that one user cannot access metrics of another user.

"Compute Unit" in the current context has a wider scope. It can be a batch job in HPC,
a VM in cloud, a pod in k8s, *etc*. The main objective of the repository is to quantify
the energy consumed and estimate emissions by each "compute unit". The repository itself
does not provide any frontend apps to show dashboards and it is meant to use along
with Grafana and Prometheus to show statistics to users.

Although CEEMS was born out of a need to monitor energy and carbon footprint of compute
workloads, it supports monitoring performance metrics as well. In addition, it leverages
[eBPF](https://ebpf.io/what-is-ebpf/) framework to monitor IO and network metrics
in a resource manager agnostic way.

## Features

- Monitor energy, performance, IO and network metrics for different types of resource
managers (SLURM, Openstack, k8s)
- Support NVIDIA (MIG and vGPU) and AMD GPUs
- Provides targets using [HTTP Discovery Component](https://grafana.com/docs/alloy/latest/reference/components/discovery/discovery.http/)
to [Grafana Alloy](https://grafana.com/docs/alloy/latest) to continuously profile compute units
- Realtime access to metrics *via* Grafana dashboards
- Access control to Prometheus datasource in Grafana
- Stores aggregated metrics in a separate DB that can be retained for long time
- CEEMS apps are [capability aware](https://tbhaxor.com/understanding-linux-capabilities/)

## Install CEEMS

> [!WARNING]
> DO NOT USE pre-release versions as the API has changed quite a lot between the
pre-release and stable versions.

Installation instructions of CEEMS components can be found in
[docs](https://mahendrapaipuri.github.io/ceems/docs/category/installation).

## Demo

<p><a href="http://195.220.87.159:30000/dashboards" target="_blank">
<img src="https://raw.githubusercontent.com/mahendrapaipuri/ceems/main/website/static/img/dashboards/demo_screenshot.png" alt="Access Demo">
</a></p>

Openstack and SLURM have been deployed on a small cloud instance and monitored using
CEEMS. As neither RAPL nor IPMI readings are available on cloud instances, energy
consumption is estimated by assuming a Thermal Design Power (TDP) value and current
usage of the instance. Several dashboards have been created in Grafana for visualizing
metrics which are listed below.

- [Overall usage of cluster](http://195.220.87.159:30000/d/adrenju36n2tcb/cluster-status?orgId=1&from=now-24h&to=now&var-job=openstack&var-host=$__all&var-provider=rte&var-country_code=FR&refresh=15m)
- [Usage of different Projects/Accounts by SLURM and Openstack](http://195.220.87.159:30000/d/cdreu45pp9erkd/user-and-project-stats?orgId=1&from=now-90d&to=now&refresh=15m)
- [Usage of Openstack resources by a given user and project](http://195.220.87.159:30000/d/be5x3it7gpx4wf/openstack-instance-summary?orgId=1&from=now-90d&to=now&var-user=gazoo&var-account=cornerstone&refresh=15m)
- [Usage of SLURM resources by a given user and project](http://195.220.87.159:30000/d/fdsm8aom8hqf4fewfwe3123dascdsc/slurm-job-summary?orgId=1&from=now-90d&to=now&var-user=wilma&var-account=bedrock&refresh=15m)

> [!WARNING]
> All the dashboards provided in the demo instance are only meant to be for demonstrative
purposes. They should not be used in production without properly protecting datasources.

## Visualizing metrics with Grafana

CEEMS is meant to be used with Grafana for visualization and below are some of the
screenshots of dashboards.

### Time series compute unit CPU metrics

<p align="center">
  <img src="https://raw.githubusercontent.com/mahendrapaipuri/ceems/main/website/static/img/dashboards/cpu_ts_stats.png" width="1200">
</p>

### Time series compute unit GPU metrics

<p align="center">
  <img src="https://raw.githubusercontent.com/mahendrapaipuri/ceems/main/website/static/img/dashboards/gpu_ts_stats.png" width="1200">
</p>

### List of compute units of user with aggregate metrics

<p align="center">
  <img src="https://raw.githubusercontent.com/mahendrapaipuri/ceems/main/website/static/img/dashboards/job_list_user.png" width="1200">
</p>

### Aggregate usage metrics of a user

<p align="center">
  <img src="https://raw.githubusercontent.com/mahendrapaipuri/ceems/main/website/static/img/dashboards/agg.png" width="1200">
</p>

## Talks and Demos

- [An Introduction to CEEMS at ISC 2024](https://drive.google.com/file/d/1kUbD3GgDKwzgIuxjrTY95YJN5aSuIejQ/view?usp=drive_link)
- [CEEMS Architecture and Usage](https://docs.google.com/presentation/d/1xNQTCsmPUz37KDb2BLrpWExuQWxk49NpVN9VDbxSe6Y/edit#slide=id.p)

## Contributing

We welcome contributions to this project, we hope to see this project grow and become
a useful tool for people who are interested in the energy and carbon footprint of their
workloads.

Please feel free to open issues and/or discussions for any potential ideas of
improvement.
