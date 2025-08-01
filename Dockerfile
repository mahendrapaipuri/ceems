ARG ARCH="amd64"
ARG OS="linux"
FROM --platform=${OS}/${ARCH} alpine:3
LABEL maintainer="Mahendra Paipuri <mahendra.paipuri@gmail.com>"

ARG ARCH="amd64"
ARG OS="linux"

# Liveness probes will use the following script
COPY build/scripts/liveness-probe.sh /bin/liveness-probe.sh

COPY .build/${OS}-${ARCH}/ceems_exporter /bin/ceems_exporter
COPY .build/${OS}-${ARCH}/ceems_api_server /bin/ceems_api_server
COPY .build/${OS}-${ARCH}/ceems_lb /bin/ceems_lb
COPY .build/${OS}-${ARCH}/ceems_k8s_admission_controller /bin/ceems_k8s_admission_controller
COPY .build/${OS}-${ARCH}/redfish_proxy /bin/redfish_proxy
COPY .build/${OS}-${ARCH}/ceems_tool /bin/ceems_tool
COPY .build/${OS}-${ARCH}/cacct /bin/cacct
COPY build/config/ceems_exporter/redfish_exporter_config.yml /etc/ceems_exporter/redfish_config.yml
COPY build/config/ceems_exporter/ebpf_profiling_config.yml /etc/ceems_exporter/ebpf_profiling_config.yml
COPY build/config/ceems_api_server/ceems_api_server.yml /etc/ceems_api_server/config.yml
COPY build/config/ceems_lb/ceems_lb.yml /etc/ceems_lb/config.yml
COPY build/config/redfish_proxy/redfish_proxy.yml /etc/redfish_proxy/config.yml
COPY build/config/cacct/cacct.yml /etc/ceems/config.yml
COPY LICENSE /LICENSE

ENV CEEMS_API_SERVER_CONFIG_FILE /etc/ceems_api_server/config.yml
ENV CEEMS_LB_CONFIG_FILE /etc/ceems_lb/config.yml
ENV REDFISH_PROXY_CONFIG_FILE /etc/redfish_proxy/config.yml

RUN mkdir -p /var/lib/ceems && chown -R root:root /etc/ceems_exporter /var/lib/ceems /etc/ceems_api_server /etc/ceems_lb /etc/redfish_proxy && chmod +x /bin/liveness-probe.sh

USER        root
WORKDIR     /var/lib/ceems
