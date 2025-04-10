---
sidebar_position: 2
---

# HPC Platforms

This document outlines reference deployments of CEEMS alongside Prometheus and Grafana on typical HPC platforms.

## Using Thanos

![CEEMS with Thanos](/img/deployment/with_thanos.png)

### Key Features

- Thanos is used for data replication and long-term storage of TSDB data
- [Litestream](https://litestream.io/) is used to replicate and create snapshots of the CEEMS API server SQLite database
- [Minio](https://min.io/) is used to store TSDB data on typical parallel file systems like Lustre, Spectrum Scale, BGFS, _etc._ in HPC platforms
- CEEMS load balancer is used to enforce access control on TSDB backends

## Using Prometheus' remote write

![CEEMS with Prometheus' remote write](/img/deployment/with_remote_write.jpg)

### Key Features

- A second instance of Prometheus with its remote write protocol is used for data replication and long-term storage
- [Litestream](https://litestream.io/) is used to replicate and create snapshots of the CEEMS API server SQLite database
- CEEMS load balancer is used to enforce access control on TSDB backends
