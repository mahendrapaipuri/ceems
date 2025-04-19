---
sidebar_position: 2
---

# Installing from Pre-compiled Binaries

Pre-compiled binaries for various architectures are distributed on the
[GitHub releases page](https://github.com/@ceemsOrg@/@ceemsRepo@/releases).

## Bash Script

A bash script is provided to install the CEEMS components using a single command:

```bash
curl -sfL https://raw.githubusercontent.com/@ceemsOrg@/@ceemsRepo@/refs/heads/main/scripts/install.sh | PREFIX=/usr/local bash -s
```

The above command will install the latest version of all CEEMS components in
`/usr/local/bin` and config files in `/usr/local/etc`.

- If a specific version is desired, use the environment variable `VERSION` to specify the version.
- If only certain components are desired, use the environment variable `APPS` to specify the
components delimited by spaces. For instance, if `ceems_api_server` and `ceems_lb` are
needed, set `APPS="ceems_api_server ceems_lb"` in the installation command.

For example, to install latest version of only `ceems_exporter`, the command will be:

```bash
curl -sfL https://raw.githubusercontent.com/@ceemsOrg@/@ceemsRepo@/refs/heads/main/scripts/install.sh | VERSION=@ceemsVersion@ APPS=ceems_exporter PREFIX=/usr/local bash -s
```

## Go Install

The CEEMS components can be installed using the `go install` command if Go version `@goVersion@` or later is available on the host. For instance, the latest version of `ceems_exporter` can
be installed as follows:

```bash
go install github.com/@ceemsOrg@/@ceemsRepo@/cmd/ceems_exporter@v@ceemsVersion@
```

Similarly, to install `ceems_api_server` or `ceems_lb`, the command will be:

```bash
go install github.com/@ceemsOrg@/@ceemsRepo@/cmd/ceems_api_server@v@ceemsVersion@
go install github.com/@ceemsOrg@/@ceemsRepo@/cmd/ceems_lb@v@ceemsVersion@
```

## Manual Install

The binaries can be manually downloaded and installed to the desired location.

```bash
wget https://github.com/@ceemsOrg@/@ceemsRepo@/releases/download/v@ceemsVersion@/ceems-@ceemsVersion@.linux-amd64.tar.gz
```

The above command will download version `@ceemsVersion@` of the CEEMS components, which can
be extracted and installed to the desired location.
