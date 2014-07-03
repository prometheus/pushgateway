FROM       ubuntu
MAINTAINER Prometheus Team <prometheus-developers@googlegroups.com>
EXPOSE     8080
WORKDIR    /pushgateway
ENTRYPOINT [ "/pushgateway/bin/pushgateway" ]

RUN        apt-get -qy update && apt-get install -yq make git curl sudo mercurial vim-common
ADD        . /pushgateway
RUN        make bin/pushgateway
