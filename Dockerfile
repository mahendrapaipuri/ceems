ARG ARCH="amd64"
ARG OS="linux"
FROM --platform=${OS}/${ARCH} alpine:3
LABEL maintainer="Mahendra Paipuri <mahendra.paipuri@gmail.com>"

ARG ARCH="amd64"
ARG OS="linux"
COPY .build/${OS}-${ARCH}/ceems_exporter /bin/ceems_exporter
COPY .build/${OS}-${ARCH}/ceems_api_server /bin/ceems_api_server
COPY .build/${OS}-${ARCH}/ceems_lb /bin/ceems_lb
COPY build/config/ceems_api_server/ceems_api_server.yml /etc/ceems_api_server/config.yml
COPY build/config/ceems_lb/ceems_lb.yml /etc/ceems_lb/config.yml
COPY LICENSE /LICENSE

ENV CEEMS_API_SERVER_CONFIG_FILE /etc/ceems_api_server/config.yml
ENV CEEMS_LB_CONFIG_FILE /etc/ceems_lb/config.yml

RUN mkdir -p /var/lib/ceems && chown -R root:root /var/lib/ceems /etc/ceems_api_server /etc/ceems_lb

USER        root
WORKDIR     /var/lib/ceems
