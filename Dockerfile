FROM       ubuntu
MAINTAINER The Prometheus Authors <prometheus-developers@googlegroups.com>
EXPOSE     9091

RUN        apt-get -qy update && apt-get install -yq make git curl sudo mercurial gcc

ADD        . /pushgateway
WORKDIR    /pushgateway
RUN        make
ENTRYPOINT [ "/pushgateway/pushgateway" ]
