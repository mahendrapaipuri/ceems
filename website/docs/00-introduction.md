---
id: intro
title: Introduction
slug: /
---

# Compute Energy & Emissions Monitoring Stack (CEEMS)
<!-- markdown-link-check-disable -->

|         |                                                                                                                                                                                                                                                                                                                                                                                                                 |
| ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| CI/CD   | [![ci](https://github.com/@ceemsOrg@/@ceemsRepo@/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/@ceemsOrg@/@ceemsRepo@/actions/workflows/ci.yml?query=branch%3Amain) [![CircleCI](https://dl.circleci.com/status-badge/img/circleci/UVxmfk5AT3EHZpsg3FdfaR/PKvLaAH1ahBZf8kBKHhCiA/tree/main.svg?style=svg&circle-token=CCIPRJ_67vq2cGkBpm9syySEp7tTW_7d4d6f3e8d72486acf477768f4f0a1d5235ab2a0)](https://dl.circleci.com/status-badge/redirect/circleci/UVxmfk5AT3EHZpsg3FdfaR/PKvLaAH1ahBZf8kBKHhCiA/tree/main)  [![Coverage](https://img.shields.io/badge/Coverage-77.6%25-brightgreen)](https://github.com/@ceemsOrg@/@ceemsRepo@/actions/workflows/ci.yml?query=branch%3Amain)                                                                                          |
| Docs    | [![docs](https://img.shields.io/badge/docs-passing-green?style=flat&link=https://github.com/@ceemsOrg@/@ceemsRepo@/blob/main/README.md)](https://github.com/@ceemsOrg@/@ceemsRepo@/blob/main/README.md)  [![Go Doc](https://godoc.org/github.com/@ceemsOrg@/@ceemsRepo@?status.svg)](http://godoc.org/github.com/@ceemsOrg@/@ceemsRepo@)                                                                                                                                                                                                                             |
| Package | [![Release](https://img.shields.io/github/v/release/@ceemsOrg@/@ceemsRepo@.svg?include_prereleases)](https://github.com/@ceemsOrg@/@ceemsRepo@/releases/latest)                                                                                                                                                                     |
| Meta    | [![GitHub License](https://img.shields.io/github/license/@ceemsOrg@/@ceemsRepo@)](https://github.com/@ceemsOrg@/@ceemsRepo@) [![Go Report Card](https://goreportcard.com/badge/github.com/@ceemsOrg@/@ceemsRepo@)](https://goreportcard.com/report/github.com/@ceemsOrg@/@ceemsRepo@) [![code style](https://img.shields.io/badge/code%20style-gofmt-blue.svg)](https://pkg.go.dev/cmd/gofmt) |

<!-- markdown-link-check-enable -->

:::warning[WARNING]

CEEMS is in early development phase, thus subject to breaking changes with no guarantee
of backward compatibility.

:::

## Features

- Monitors energy, performance, IO and network metrics for different types of resource
managers (SLURM, Openstack, k8s)
- Supports different energy sources like RAPL, HWMON, Cray's PM Counters and BMC _via_ IPMI or Redfish
- Supports NVIDIA (MIG, time sharing, MPS and vGPU) and AMD GPUs ([Partition](https://rocm.blogs.amd.com/software-tools-optimization/compute-memory-modes/README.html) like CPX, QPX, TPX, DPX)
- Supports zero instrumentation eBPF based continuous profiling using
[Grafana Pyroscope](https://grafana.com/oss/pyroscope/) as backend
- Realtime access to metrics _via_ Grafana dashboards or a simple CLI tool
- Multi-tenancy and access control to Prometheus and Pyroscope datasources in Grafana
- Stores aggregated metrics in a separate DB that can be retained for long time
- CEEMS apps are [capability aware](https://tbhaxor.com/understanding-linux-capabilities/)

## Components

CEEMS provide a set of components that enable operators and end users to monitor the consumption of
resources of the compute units of different resource managers like SLURM, Openstack and
Kubernetes.

- CEEMS Prometheus exporter is capable of exporting compute unit metrics including energy
consumption, performance, IO and network metrics from different resource managers in a
unified manner. In addition, CEEMS exporter is capable of continuous profiling of compute units using
[eBPF](https://grafana.com/docs/pyroscope/latest/configure-client/grafana-alloy/ebpf/)

- CEEMS API server can store the aggregate metrics and metadata of each compute unit
originating from different resource managers.

- CEEMS load balancer provides basic access control on TSDB and Pyroscope so that compute unit metrics
from different projects/tenants/namespaces are isolated.

"Compute Unit" in the current context has a wider scope. It can be a batch job in HPC,
a VM in cloud, a pod in k8s, _etc_. The main objective of the stack is to quantify
the energy consumed and estimate emissions by each "compute unit". The repository itself
does not provide any frontend apps to show dashboards and it is meant to use along
with Grafana and Prometheus to show statistics to users.
