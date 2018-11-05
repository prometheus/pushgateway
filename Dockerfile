FROM golang:latest as base

RUN mkdir -p /go/src/github.com/prometheus/pushgateway
WORKDIR /go/src/github.com/prometheus/pushgateway

COPY . .

RUN make

FROM scratch
LABEL maintainer="The Prometheus Authors <prometheus-developers@googlegroups.com>"

EXPOSE 9091

COPY --from=base /go/src/github.com/prometheus/pushgateway/pushgateway /

ENTRYPOINT ["/pushgateway"]

