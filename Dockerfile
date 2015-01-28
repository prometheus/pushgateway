FROM       ubuntu
MAINTAINER Prometheus Team <prometheus-developers@googlegroups.com>
EXPOSE     9091
WORKDIR    /pushgateway
ENTRYPOINT [ "/pushgateway/bin/pushgateway" ]

RUN        apt-get -qy update && apt-get install -yq make git curl sudo mercurial gcc
ADD        . /pushgateway
RUN        make bin/pushgateway
