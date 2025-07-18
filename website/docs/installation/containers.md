---
sidebar_position: 4
---

# Containers

CEEMS is distributed as container images as well, published on
[DockerHub](https://hub.docker.com/r/@ceemsContOrg@/@ceemsRepo@)
and [Quay](https://quay.io/repository/@ceemsContOrg@/@ceemsRepo@). All CEEMS components
are distributed in a single container image.

Container images are published for every released version using the release version as
the container tag. Besides, the `main` branch is also published with the `main` tag. The
`latest` tag always points to the latest stable release.

## Pulling Container

Container images can be pulled from either DockerHub or Quay:

```bash
docker pull @ceemsContOrg@/@ceemsRepo@:@ceemsVersion@
# or
docker pull quay.io/@ceemsContOrg@/@ceemsRepo@:@ceemsVersion@
```

## Running Container

The container can be run by using the appropriate app as the command for the container.
For instance, to run `ceems_exporter` using a container, it is run as follows:

```bash
docker run @ceemsContOrg@/@ceemsRepo@:@ceemsVersion@ ceems_exporter <CLI args>
```

where `<CLI args>` are the command-line arguments for `ceems_exporter`.
