# Compute Energy & Emissions Monitoring Stack (CEEMS)
<!-- markdown-link-check-disable -->

|         |                                                                                                                                                                                                                                                                                                                                                                                                                 |
| ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| CI/CD   | [![ci](https://github.com/mahendrapaipuri/ceems/workflows/CI/badge.svg)](https://github.com/mahendrapaipuri/ceems) [![CircleCI](https://dl.circleci.com/status-badge/img/circleci/8jSYT1wyKY8mKQRTqNLThX/TzM1Mr3AEAqmehnoCde19R/tree/main.svg?style=svg&circle-token=28db7268f3492790127da28e62e76b0991d59c8b)](https://dl.circleci.com/status-badge/redirect/circleci/8jSYT1wyKY8mKQRTqNLThX/TzM1Mr3AEAqmehnoCde19R/tree/main)  [![Coverage](https://img.shields.io/badge/Coverage-73.9%25-brightgreen)](https://github.com/mahendrapaipuri/ceems/actions/workflows/ci.yml?query=branch%3Amain)                                                                                          |
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
a VM in cloud, a pod in k8s, _etc_. The main objective of the repository is to quantify
the energy consumed and estimate emissions by each "compute unit". The repository itself
does not provide any frontend apps to show dashboards and it is meant to use along
with Grafana and Prometheus to show statistics to users.

## Install CEEMS

> [!WARNING] 
> DO NOT USE pre-release versions as the API has changed quite a lot between the 
pre-release and stable versions.

Installation instructions of CEEMS components can be found in 
[docs](https://mahendrapaipuri.github.io/ceems/docs/category/installation).

## Visualizing metrics with Grafana

CEEMS is meant to be used with Grafana for visualization and below are some of the 
screenshots few possible metrics.

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
