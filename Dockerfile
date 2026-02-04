ARG ARCH="amd64"
ARG OS="linux"
FROM quay.io/prometheus/busybox-${OS}-${ARCH}:latest
LABEL maintainer="The Prometheus Authors <prometheus-developers@googlegroups.com>"

ARG ARCH="amd64"
ARG OS="linux"
COPY --chown=nobody:nobody .build/${OS}-${ARCH}/pushgateway /bin/pushgateway

EXPOSE 9091
WORKDIR /pushgateway
RUN chown nobody:nobody /pushgateway && chmod g+w /pushgateway

USER 65534

ENTRYPOINT [ "/bin/pushgateway" ]
