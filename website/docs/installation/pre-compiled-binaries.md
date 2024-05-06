---
sidebar_position: 2
---

# Installing from Pre-compiled binaries

Pre-compiled binaries for various architectures are distributed in the 
[GitHub releases page](https://github.com/mahendrapaipuri/ceems/releases).

## Bash script

A bash script is provided to install the CEEMS components using a single command

```
curl -sfL https://github.com/mahendrapaipuri/ceems/blob/main/scripts/install.sh | PREFIX=/usr/local bash -s
```

The above command will install the latest version of all CEEMS components in 
`/usr/local/bin` and config files in `/usr/local/etc`. 

- If specific version is desired, use environment variable `VERSION` to specify the version.
- If only certain components are desired, use environment variable `APPS` to specify the 
components delimited by space. For instance, if `ceems_api_server` and `ceems_lb` are 
needed, set `APPS="ceems_api_server ceems_lb"` to the installation command.

## Go install

The binaries can be installed using `go install` command provided that `go >= 1.21.x` 
is available on ths host. For instance, the latest version of `ceems_exporter` can 
be installed as follows:

```
go install github.com/mahendrapaipuri/ceems/cmd/ceems_exporter@latest
```

Similarly, to install `ceems_api_server` or `ceems_lb`, the command will be

```
go install github.com/mahendrapaipuri/ceems/cmd/{ceems_api_server,ceems_lb}@latest
```

## Manual install

The binaries can be manually downloaded and installed to desired location.

```
wget https://github.com/mahendrapaipuri/ceems/releases/download/v0.1.0/ceems-0.1.0.linux-amd64.tar.gz
```

The above command will download the `0.1.0` version of CEEMS components and they 
can be extracted and installed to desired location.
