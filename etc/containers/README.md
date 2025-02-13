# Podman Quadlets

This folder contains the [Quadlet](https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html)
files for Podman to deploy CEEMS Exporter and NVIDIA DCGM exporter as containers.
These must be installed at `/etc/containers/systemd` on the host. Once the systemd
has been reloaded using `systemctl daemon-reload`, Systemd will generate individual
service files for each Quadlet and manages the life cycle of the containers.
