FROM quay.io/prometheus/busybox:latest
LABEL maintainer="The Prometheus Authors <prometheus-developers@googlegroups.com>"

COPY --chown=nobody:nogroup pushgateway /bin/pushgateway

EXPOSE 9091
RUN mkdir -p /pushgateway && chown nobody:nogroup /pushgateway
WORKDIR /pushgateway

USER 65534

ENTRYPOINT [ "/bin/pushgateway" ]
