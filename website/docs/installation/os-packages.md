---
sidebar_position: 3
---

# Installing from OS Packages

CEEMS provides RPM and DEB packages for installation on RedHat and Debian variant OS
distributions. The packages are available on the
[GitHub releases page](https://github.com/@ceemsOrg@/@ceemsRepo@/releases).

## RPM Package

The package can be downloaded from the releases page and installed using either the `yum` or
`dnf` command. For example, to install `ceems_exporter`:

```bash
wget https://github.com/@ceemsOrg@/@ceemsRepo@/releases/download/v@ceemsVersion@/ceems_exporter-@ceemsVersion@-linux-amd64.rpm
yum install ./ceems_exporter-@ceemsVersion@-linux-amd64.rpm
```

## DEB Package

Similarly, on Debian systems, each component can be installed by downloading the
DEB package from the releases page:

```bash
wget https://github.com/@ceemsOrg@/@ceemsRepo@/releases/download/v@ceemsVersion@/ceems_exporter-@ceemsVersion@-linux-amd64.deb
apt install ./ceems_exporter-@ceemsVersion@-linux-amd64.deb
```

This example will install `ceems_exporter` in `/usr/local/bin`, config files in
`/etc/ceems_exporter` directory, and install a basic systemd unit file that enables and
starts the service
