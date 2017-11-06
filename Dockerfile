# Intermediary Build Container
FROM golang:1.9

RUN mkdir -p /go/src/github.com/monzo/pushgateway
WORKDIR /go/src/github.com/monzo/pushgateway
COPY . .
RUN go get
RUN make assets
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o pushgateway .


# Final (static from scratch container)
FROM scratch

ARG VCS_REF
ARG BUILD_DATE

MAINTAINER Charlie Gildawie (charlieg@monzo.com)
LABEL org.label-schema.name="pushgateway" \
      org.label-schema.description="Prometheus Push Endpoint for collection" \
      org.label-schema.build-date=$BUILD_DATE \
      org.label-schema.vcs-ref=$VCS_REF \
      org.label-schema.vcs-url="https://github.com/monzo/pushgateway"

COPY --from=0 /go/src/github.com/monzo/pushgateway/pushgateway /pushgateway

EXPOSE 9091
ENTRYPOINT ["/pushgateway"]
