---
sidebar_position: 6
---

# Systemd

If CEEMS components are installed using [RPM/DEB packages](../installation/os-packages.md), a basic 
systemd unit file will be installed to start the service. However, when they are 
installed manually using [pre-compiled binaries](../installation/pre-compiled-binaries.md), it is 
necessary to install and configure `systemd` unit files to manage the service.

## Privileges

CEEMS exporter executes IPMI DCMI command which is a privileged command. Thus, a trivial
solution is to run CEEMS exporter as `root`. Similarly, in order to which GPU has been 
assigned to which compute unit, we need to either introspect compute unit's 
environment variables (for SLURM resource manager), `libvirt` XML file for VM (for Openstack),
_etc_. These actions need privileges as well. However, most of the other collectors of 
CEEMS exporter do not need additional privileges and hence, running the exporter 
as `root` is an overkill. We can leverage 
[Linux Capabilities](https://man7.org/linux/man-pages/man7/capabilities.7.html) to 
assign just the necessary privileges to the process.

## Linux capabilities

Linux capabilities can be assigned to either file or process. For instance, capabilities 
on the `ceems_exporter` and `ceems_api_server` binaries can be set as follows:

```
sudo setcap cap_sys_ptrace,cap_dac_read_search,cap_setuid,cap_setgid+ep /full/path/to/ceems_exporter
sudo setcap cap_setuid,cap_setgid+ep /full/path/to/ceems_api_server
```

This will assign all the capabilities that are necessary to run `ceems_exporter` 
for all the collectors. Using file based capabilities will 
expose those capabilities to anyone on the system that have execute permissions on the 
binary. Although, it does not pose a big security concern, it is better to assign 
capabilities to a process. 

As operators tend to run the exporter within a `systemd` unit file, we can assign 
capabilities to the process rather than file using `AmbientCapabilities` 
directive of the `systemd`. An example is as follows:

```
[Service]
ExecStart=/usr/local/bin/ceems_exporter
AmbientCapabilities=CAP_SYS_PTRACE CAP_DAC_READ_SEARCH CAP_SETUID CAP_SETGID
```

Note that it is bare minimum service file and it is only to demonstrate on how to use 
`AmbientCapabilities`. Production ready [service files examples]((https://github.com/mahendrapaipuri/ceems/tree/main/build/package)) 
are provided in repo.
