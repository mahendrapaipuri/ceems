---
id: intro
title: Introduction
slug: /
---

# Compute Energy & Emissions Monitoring Stack (CEEMS)
<!-- markdown-link-check-disable -->

|         |                                                                                                                                                                                                                                                                                                                                                                                                                 |
| ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| CI/CD   | [![ci](https://github.com/mahendrapaipuri/ceems/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/mahendrapaipuri/ceems/actions/workflows/ci.yml?query=branch%3Amain) [![CircleCI](https://dl.circleci.com/status-badge/img/circleci/8jSYT1wyKY8mKQRTqNLThX/TzM1Mr3AEAqmehnoCde19R/tree/main.svg?style=svg&circle-token=28db7268f3492790127da28e62e76b0991d59c8b)](https://dl.circleci.com/status-badge/redirect/circleci/8jSYT1wyKY8mKQRTqNLThX/TzM1Mr3AEAqmehnoCde19R/tree/main)  [![Coverage](https://img.shields.io/badge/Coverage-75.9%25-brightgreen)](https://github.com/mahendrapaipuri/ceems/actions/workflows/ci.yml?query=branch%3Amain)                                                                                          |
| Docs    | [![docs](https://img.shields.io/badge/docs-passing-green?style=flat&link=https://github.com/mahendrapaipuri/ceems/blob/main/README.md)](https://github.com/mahendrapaipuri/ceems/blob/main/README.md)                                                                                                                                                                                                                               |
| Package | [![Release](https://img.shields.io/github/v/release/mahendrapaipuri/ceems.svg?include_prereleases)](https://github.com/mahendrapaipuri/ceems/releases/latest)                                                                                                                                                                     |
| Meta    | [![GitHub License](https://img.shields.io/github/license/mahendrapaipuri/ceems)](https://github.com/mahendrapaipuri/ceems) [![Go Report Card](https://goreportcard.com/badge/github.com/mahendrapaipuri/ceems)](https://goreportcard.com/report/github.com/mahendrapaipuri/ceems) [![code style](https://img.shields.io/badge/code%20style-gofmt-blue.svg)](https://pkg.go.dev/cmd/gofmt) |

<!-- markdown-link-check-enable -->

:::warning[WARNING]

CEEMS is in early development phase, thus subject to breaking changes with no guarantee
of backward compatibility.

:::

CEEMS provide a set of components that enable operators to monitor the consumption of
resources of the compute units of different resource managers like SLURM, Openstack and
Kubernetes.

- CEEMS Prometheus exporter is capable of exporting compute unit metrics including energy
consumption, performance, IO and network metrics from different resource managers in a
unified manner.

- CEEMS API server can store the aggregate metrics and metadata of each compute unit
originating from different resource managers.

- CEEMS load balancer provides basic access control on TSDB so that compute unit metrics
from different projects/tenants/namespaces are isolated.

"Compute Unit" in the current context has a wider scope. It can be a batch job in HPC,
a VM in cloud, a pod in k8s, _etc_. The main objective of the stack is to quantify
the energy consumed and estimate emissions by each "compute unit". The repository itself
does not provide any frontend apps to show dashboards and it is meant to use along
with Grafana and Prometheus to show statistics to users.

:::important[Note]

Currently, only SLURM is supported as a resource manager. In future support for Openstack
and Kubernetes will be added.

:::
