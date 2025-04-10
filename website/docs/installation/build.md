---
sidebar_position: 6
---

# Build from Source

To build the applications from source, Go `1.21.x` must be installed on the 
host. The installation of Go can be found in the [Go documentation](https://go.dev/doc/install). Once 
Go is installed and added to the `PATH`, a `Makefile` is provided in the repository
to build CEEMS components.

First, we need to clone the repository:

```bash
git clone https://github.com/mahendrapaipuri/ceems.git
```

## CEEMS Exporter

Once the CEEMS repository is cloned, building the CEEMS exporter can be done as follows:

```bash
cd ceems
make build
```

This command will build `ceems_exporter` and place it in the `./bin` folder in the 
current directory.

## CEEMS API Server and CEEMS Load Balancer

The CEEMS API server and CEEMS load balancer use SQLite and hence require CGO for building. 
To build these two components, execute:

```bash
CGO_BUILD=1 make build
```

This will build the `ceems_api_server` and `ceems_lb` binaries in the `./bin` folder.

