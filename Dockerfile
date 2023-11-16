ARG ARCH="amd64"
ARG OS="linux"
FROM quay.io/prometheus/busybox-${OS}-${ARCH}:latest
LABEL maintainer="Mahendra Paipuri <mahendra.paipuri@gmail.com>"

ARG ARCH="amd64"
ARG OS="linux"
COPY .build/${OS}-${ARCH}/batchjob_exporter /bin/batchjob_exporter

EXPOSE      9100
USER        nobody
ENTRYPOINT  [ "/bin/batchjob_exporter" ]
