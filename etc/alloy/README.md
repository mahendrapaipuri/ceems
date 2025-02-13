# Grafana Alloy

This is a basic configuration file for Grafana Alloy to work along
with CEEMS Exporter. By Grafana Alloy spins up a local server running
at `127.0.0.1:12345` which can be used for debugging purposes. On
a multi-user environment like HPC platforms, it is prohibitive to give
access to this Alloy server to the end users. Thus, we leverage mTLS
to protect the Alloy server with TLS certificates.
