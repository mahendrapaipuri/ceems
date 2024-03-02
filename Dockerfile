ARG ARCH="amd64"
ARG OS="linux"
FROM quay.io/prometheus/busybox-${OS}-${ARCH}:latest
LABEL maintainer="Mahendra Paipuri <mahendra.paipuri@gmail.com>"

ARG ARCH="amd64"
ARG OS="linux"
COPY .build/${OS}-${ARCH}/ceems_exporter /bin/ceems_exporter
COPY .build/${OS}-${ARCH}/ceems_api_server /bin/ceems_api_server
COPY .build/${OS}-${ARCH}/ceems_lb /bin/ceems_lb
COPY LICENSE /LICENSE

USER        nobody
WORKDIR     /bin
