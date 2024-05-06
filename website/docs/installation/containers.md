---
sidebar_position: 4
---

# Containers

CEEMS is distributed as container images as well published on 
[DockerHub](https://hub.docker.com/r/mahendrapaipuri/ceems)
and [Quay](https://quay.io/repository/mahendrapaipuri/ceems). All the CEEMS components 
are distributed in a single container image.

Container images are published for every released version using release version as 
container tag. Besides, the `main` branch is also published with `main` tag. The 
`latest` tag always points to the latest stable release.

## Pulling container

Container images can be pulled either from DockerHub or Quay using 

```bash
docker pull mahendrapaipuri/ceems:latest
# or
docker pull quay.io/mahendrapaipuri/ceems:latest
```

## Running container

The container can be run by using appropriate app as the command to the container. 
For instance, for running `ceems_exporter` using container, it is run as follows:

```
docker run mahendrapaipuri/ceems:latest ceems_exporter <CLI args>
```

where `<CLI args>` are the command line arguments for `ceems_exporter`.
