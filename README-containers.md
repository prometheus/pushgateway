# Prometheus Pushgateway

[![CircleCI](https://circleci.com/gh/prometheus/pushgateway/tree/master.svg?style=shield)][circleci]
[![Docker Repository on Quay](https://quay.io/repository/prometheus/pushgateway/status)][quay]
[![Docker Pulls](https://img.shields.io/docker/pulls/prom/pushgateway.svg?maxAge=604800)][hub]

The Prometheus Pushgateway exists to allow ephemeral and batch jobs to
expose their metrics to Prometheus. Since these kinds of jobs may not
exist long enough to be scraped, they can instead push their metrics
to a Pushgateway. The Pushgateway then exposes these metrics to
Prometheus.

## Non-goals

First of all, the Pushgateway is not capable of turning Prometheus into a
push-based monitoring system. For a general description of use cases for the
Pushgateway, please read [When To Use The
Pushgateway](https://prometheus.io/docs/practices/pushing/).

The Pushgateway is explicitly not an _aggregator or distributed counter_ but
rather a metrics cache. It does not have
[statsd](https://github.com/etsy/statsd)-like semantics. The metrics pushed are
exactly the same as you would present for scraping in a permanently running
program. If you need distributed counting, you could either use the actual
statsd in combination with the [Prometheus statsd
exporter](https://github.com/prometheus/statsd_exporter), or have a look at the
[prom-aggregation-gateway](https://github.com/zapier/prom-aggregation-gateway). With more
experience gathered, the Prometheus project might one day be able to provide a
native solution, separate from or possibly even as part of the Pushgateway.

For machine-level metrics, the
[textfile](https://github.com/prometheus/node_exporter/blob/master/README.md#textfile-collector)
collector of the Node exporter is usually more appropriate. The Pushgateway is
intended for service-level metrics.

The Pushgateway is not an _event store_. While you can use Prometheus as a data
source for
[Grafana annotations](http://docs.grafana.org/reference/annotations/), tracking
something like release events has to happen with some event-logging framework.

A while ago, we
[decided to not implement a “timeout” or TTL for pushed metrics](https://github.com/prometheus/pushgateway/issues/19)
because almost all proposed use cases turned out to be anti-patterns we
strongly discourage. You can follow a more recent discussion on the
[prometheus-developers mailing list](https://groups.google.com/forum/#!topic/prometheus-developers/9IyUxRvhY7w).

## Run it

Download binary releases for your platform from the
[release page](https://github.com/prometheus/pushgateway/releases) and unpack
the tarball.

If you want to compile yourself from the sources, you need a working Go
setup. Then use the provided Makefile (type `make`).

For the most basic setup, just start the binary. To change the address
to listen on, use the `--web.listen-address` flag (e.g. "0.0.0.0:9091" or ":9091").
By default, Pushgateway does not persist metrics. However, the `--persistence.file` flag
allows you to specify a file in which the pushed metrics will be
persisted (so that they survive restarts of the Pushgateway).

### Using Docker

You can deploy the Pushgateway using the [prom/pushgateway](https://hub.docker.com/r/prom/pushgateway) Docker image.

For example:

```bash
docker pull prom/pushgateway

docker run -d -p 9091:9091 prom/pushgateway
```

## More documentation

Please refer to the [full length
README.md](https://github.com/prometheus/pushgateway/blob/master/README.md) for
more documentation.
