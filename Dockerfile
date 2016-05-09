FROM        quay.io/prometheus/busybox:latest
MAINTAINER  The Prometheus Authors <prometheus-developers@googlegroups.com>

COPY pushgateway /bin/pushgateway

EXPOSE     9091
WORKDIR    /pushgateway
ENTRYPOINT [ "/bin/pushgateway" ]
