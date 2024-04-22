ARG ARCH="amd64"
ARG OS="linux"
FROM quay.io/prometheus/busybox-${OS}-${ARCH}:latest
LABEL maintainer="Mahendra Paipuri <mahendra.paipuri@gmail.com>"

ARG ARCH="amd64"
ARG OS="linux"
COPY .build/${OS}-${ARCH}/ceems_exporter /bin/ceems_exporter
COPY .build/${OS}-${ARCH}/ceems_api_server /bin/ceems_api_server
COPY .build/${OS}-${ARCH}/ceems_lb /bin/ceems_lb
COPY build/config/ceems_api_server/tsdb-config.yml /etc/ceems_api_server/tsdb-config.yml
COPY build/config/ceems_lb/config.yml /etc/ceems_lb/config.yml
COPY LICENSE /LICENSE

RUN mkdir /ceems && chown -R nobody:nobody /ceems /etc/ceems_api_server /etc/ceems_lb

USER        nobody
WORKDIR     /ceems
