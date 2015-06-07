FROM        sdurrheimer/alpine-golang-make-onbuild
MAINTAINER  The Prometheus Authors <prometheus-developers@googlegroups.com>

USER root
RUN  mkdir /pushgateway \
     && chown golang:golang /pushgateway

USER        golang
WORKDIR     /pushgateway
EXPOSE      9091
