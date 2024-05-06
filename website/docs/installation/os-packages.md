---
sidebar_position: 3
---

# Installing from OS packages

CEEMS provides RPM and DEB packages to install on RedHat and Debian variants OS 
distributions. The packages are provided in the 
[GitHub releases page](https://github.com/mahendrapaipuri/ceems/releases).

## RPM package

The package can be downloaded from releases page and installed using either `yum` or 
`dnf` command. For example, to install `ceems_exporter` 

```
wget https://github.com/mahendrapaipuri/ceems/releases/download/v0.1.0/ceems_exporter-0.1.0-linux-amd64.rpm
yum install ./ceems_exporter-0.1.0-linux-amd64.rpm
```

## DEB package

Similarly on the debian systems, each component can be installed by downloading the 
DEB package from releases page.

```
wget https://github.com/mahendrapaipuri/ceems/releases/download/v0.1.0/ceems_exporter-0.1.0-linux-amd64.deb
apt install ./ceems_exporter-0.1.0-linux-amd64.deb
```

This example will install the `ceems_exporter` in `/usr/local/bin`, config files in 
`/etc/ceems_exporter` directory and install a basic systemd unit file that enables and
start the service
