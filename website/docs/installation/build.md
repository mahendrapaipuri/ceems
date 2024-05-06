---
sidebar_position: 6
---

# Build from source

To build the applications from the source, `go 1.21.x` must be installed on the 
host. Installation of Go can be found in [Go Docs](https://go.dev/doc/install). Once 
`go` is installed and added to the `PATH`, a `Makefile` is provided in the repository
to build CEEMS components.

First we need to clone the repository

```
git clone https://github.com/mahendrapaipuri/ceems.git
```

## CEEMS Exporter

Once CEEMS repository is cloned, build CEEMS exporter can be done as follows:

```
cd ceems
make build
```

This command will build `ceems_exporter` and place it in `./bin` folder in the 
current directory.

## CEEMS API Server and CEEMS Load Balancer

CEEMS API server and CEEMS load balancer uses SQLite and hence, we need CGO for building. 
Hence, to build these two components, we need to execute

```
CGO_BUILD=1 make build
```

This will build `ceems_api_server` and `ceems_lb` binaries in `./bin` folder.

