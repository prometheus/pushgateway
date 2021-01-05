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
exporter](https://github.com/prometheus/statsd_exporter), or have a look at
[Weavework's aggregation
gateway](https://github.com/weaveworks/prom-aggregation-gateway). With more
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

## Use it

### Configure the Pushgateway as a target to scrape

The Pushgateway has to be configured as a target to scrape by Prometheus, using
one of the usual methods. _However, you should always set `honor_labels: true`
in the scrape config_ (see [below](#about-the-job-and-instance-labels) for a
detailed explanation).

### Libraries

Prometheus client libraries should have a feature to push the
registered metrics to a Pushgateway. Usually, a Prometheus client
passively presents metric for scraping by a Prometheus server. A
client library that supports pushing has a push function, which needs
to be called by the client code. It will then actively push the
metrics to a Pushgateway, using the API described below.

### Command line

Using the Prometheus text protocol, pushing metrics is so easy that no
separate CLI is provided. Simply use a command-line HTTP tool like
`curl`. Your favorite scripting language has most likely some built-in
HTTP capabilities you can leverage here as well.

*Note that in the text protocol, each line has to end with a line-feed
character (aka 'LF' or '\n'). Ending a line in other ways, e.g. with 'CR' aka
'\r', 'CRLF' aka '\r\n', or just the end of the packet, will result in a
protocol error.*

Pushed metrics are managed in groups, identified by a grouping key of any
number of labels, of which the first must be the `job` label. The groups are
easy to inspect via the web interface.

*For implications of special characters in label values see the [URL
section](#url) below.*

Examples:

* Push a single sample into the group identified by `{job="some_job"}`:

        echo "some_metric 3.14" | curl --data-binary @- http://pushgateway.example.org:9091/metrics/job/some_job

  Since no type information has been provided, `some_metric` will be of type `untyped`.

* Push something more complex into the group identified by `{job="some_job",instance="some_instance"}`:

        cat <<EOF | curl --data-binary @- http://pushgateway.example.org:9091/metrics/job/some_job/instance/some_instance
        # TYPE some_metric counter
        some_metric{label="val1"} 42
        # TYPE another_metric gauge
        # HELP another_metric Just an example.
        another_metric 2398.283
        EOF

  Note how type information and help strings are provided. Those lines
  are optional, but strongly encouraged for anything more complex.

* Delete all metrics in the group identified by
  `{job="some_job",instance="some_instance"}`:

        curl -X DELETE http://pushgateway.example.org:9091/metrics/job/some_job/instance/some_instance

* Delete all metrics in the group identified by `{job="some_job"}` (note that
  this does not include metrics in the
  `{job="some_job",instance="some_instance"}` group from the previous example,
  even if those metrics have the same job label):

        curl -X DELETE http://pushgateway.example.org:9091/metrics/job/some_job
        
* Delete all metrics in all groups (requires to enable the admin API via the command line flag `--web.enable-admin-api`):

        curl -X PUT http://pushgateway.example.org:9091/api/v1/admin/wipe

### About the job and instance labels

The Prometheus server will attach a `job` label and an `instance` label to each
scraped metric. The value of the `job` label comes from the scrape
configuration. When you configure the Pushgateway as a scrape target for your
Prometheus server, you will probably pick a job name like `pushgateway`. The
value of the `instance` label is automatically set to the host and port of the
target scraped. Hence, all the metrics scraped from the Pushgateway will have
the host and port of the Pushgateway as the `instance` label and a `job` label
like `pushgateway`. The conflict with the `job` and `instance` labels you might
have attached to the metrics pushed to the Pushgateway is solved by renaming
those labels to `exported_job` and `exported_instance`.

However, this behavior is usually undesired when scraping a
Pushgateway. Generally, you would like to retain the `job` and `instance`
labels of the metrics pushed to the Pushgateway. That's why you have set
`honor_labels: true` in the scrape config for the Pushgateway. It enables the
desired behavior. See the
[documentation](https://prometheus.io/docs/operating/configuration/#scrape_config)
for details.

This leaves us with the case where the metrics pushed to the Pushgateway do not
feature an `instance` label. This case is quite common as the pushed metrics
are often on a service level and therefore not related to a particular
instance. Even with `honor_labels: true`, the Prometheus server will attach an
`instance` label if no `instance` label has been set in the first
place. Therefore, if a metric is pushed to the Pushgateway without an instance
label (and without instance label in the grouping key, see below), the
Pushgateway will export it with an empty instance label (`{instance=""}`),
which is equivalent to having no `instance` label at all but prevents the
server from attaching one.

### About metric inconsistencies

The Pushgateway exposes all pushed metrics together with its own metrics via
the same `/metrics` endpoint. (See the section about [exposed
metrics](#exposed-metrics) for details.) Therefore, all the metrics have to be
consistent with each other: Metrics of the same name must have
the same type, even if they are pushed to different groups, and there must be
no duplicates, i.e. metrics with the same name and the exact same label
pairs. Pushes that would lead to inconsistencies are rejected with status
code 400.

Inconsistent help strings are tolerated, though. The Pushgateway will pick a
winning help string and log about it at info level.

_Legacy note: The help string of Pushgateway's own `push_time_seconds` metric
has changed in v0.10.0. By using a persistence file, metrics pushed to a
Pushgateway of an earlier version can make it into a Pushgateway of v0.10.0 or
later. In this case, the above mentioned log message will show up. Once each
previously pushed group has been deleted or received a new push, the log
message will disappear._

The consistency check performed during a push is the same as it happens anyway
during a scrape. In common use cases, scrapes happen more often than
pushes. Therefore, the performance cost of the push-time check isn't
relevant. However, if a large amount of metrics on the Pushgateway is combined
with frequent pushes, the push duration might become prohibitively long. In
this case, you might consider using the command line flag
`--push.disable-consistency-check`, which saves the cost of the consistency
check during a push but allows pushing inconsistent metrics. The check will
still happen during a scrape, thereby failing all scrapes for as long as
inconsistent metrics are stored on the Pushgateway. Setting the flag therefore
puts you at risk to disable the Pushgateway by a single inconsistent push.

### About timestamps

If you push metrics at time *t*<sub>1</sub>, you might be tempted to believe
that Prometheus will scrape them with that same timestamp
*t*<sub>1</sub>. Instead, what Prometheus attaches as a timestamp is the time
when it scrapes the Pushgateway. Why so?

In the world view of Prometheus, a metric can be scraped at any
time. A metric that cannot be scraped has basically ceased to
exist. Prometheus is somewhat tolerant, but if it cannot get any
samples for a metric in 5min, it will behave as if that metric does
not exist anymore. Preventing that is actually one of the reasons to
use a Pushgateway. The Pushgateway will make the metrics of your
ephemeral job scrapable at any time. Attaching the time of pushing as
a timestamp would defeat that purpose because 5min after the last
push, your metric will look as stale to Prometheus as if it could not
be scraped at all anymore. (Prometheus knows only one timestamp per
sample, there is no way to distinguish a 'time of pushing' and a 'time
of scraping'.)

As there aren't any use cases where it would make sense to attach a
different timestamp, and many users attempting to incorrectly do so (despite no
client library supporting this), the Pushgateway rejects any pushes with
timestamps.

If you think you need to push a timestamp, please see [When To Use The
Pushgateway](https://prometheus.io/docs/practices/pushing/).

In order to make it easier to alert on failed pushers or those that have not
run recently, the Pushgateway will add in the metrics `push_time_seconds` and
`push_failure_time_seconds` with the Unix timestamp of the last successful and
failed `POST`/`PUT` to each group. This will override any pushed metric by that
name. A value of zero for either metric implies that the group has never seen a
successful or failed `POST`/`PUT`.

## API

All pushes are done via HTTP. The interface is vaguely REST-like.

### URL

The default port the Pushgateway is listening to is 9091. The path looks like

    /metrics/job/<JOB_NAME>{/<LABEL_NAME>/<LABEL_VALUE>}

`<JOB_NAME>` is used as the value of the `job` label, followed by any
number of other label pairs (which might or might not include an
`instance` label). The label set defined by the URL path is used as a
grouping key. Any of those labels already set in the body of the
request (as regular labels, e.g. `name{job="foo"} 42`)
_will be overwritten to match the labels defined by the URL path!_

If `job` or any label name is suffixed with `@base64`, the following job name
or label value is interpreted as a base64 encoded string according to [RFC
4648, using the URL and filename safe
alphabet](https://tools.ietf.org/html/rfc4648#section-5). (Padding is optional,
but a single `=` is required to encode an empty label value.) This is the only
way to handle the following cases:

* A job name or a label value that contains a `/`, because the plain (or even
  URI-encoded) `/` would otherwise be interpreted as a path separator.
* An empty label value, because the resulting `//` or trailing `/` would
  disappear when the path is sanitized by the HTTP router code. Note that an
  empty `job` name is invalid. Empty label values are valid but rarely
  useful. To encode them with base64, you have to use at least one `=` padding
  character to avoid a `//` or a trailing `/`.

For other special characters, the usual URI component encoding works, too, but
the base64 might be more convenient.

Ideally, client libraries take care of the suffixing and encoding.

Examples:

* To use the grouping key `job="directory_cleaner",path="/var/tmp"`, the
  following path will _not_ work:

      /metrics/job/directory_cleaner/path//var/tmp
      
  Instead, use the base64 URL-safe encoding for the label value and mark it by
  suffixing the label name with `@base64`:
  
      /metrics/job/directory_cleaner/path@base64/L3Zhci90bXA
      
  If you are not using a client library that handles the encoding for you, you
  can use encoding tools. For example, there is a command line tool `base64url`
  (Debian package `basez`), which you could combine with `curl` to push from
  the command line in the following way:
  
      echo 'some_metric{foo="bar"} 3.14' | curl --data-binary @- http://pushgateway.example.org:9091/metrics/job/directory_cleaner/path@base64/$(echo -n '/var/tmp' | base64url)

* To use a grouping key containing an empty label value such as
  `job="example",first_label="",second_label="foobar"`, the following path will
  _not_ work:
  
       /metrics/job/example/first_label//second_label/foobar

  Instead, use the following path including the `=` padding character:
  
      /metrics/job/example/first_label@base64/=/second_label/foobar

* The grouping key `job="titan",name="Προμηθεύς"` can be represented
  “traditionally” with URI encoding:
  
      /metrics/job/titan/name/%CE%A0%CF%81%CE%BF%CE%BC%CE%B7%CE%B8%CE%B5%CF%8D%CF%82
      
  Or you can use the more compact base64 encoding:
  
      /metrics/job/titan/name@base64/zqDPgc6_zrzOt864zrXPjc-C

### `PUT` method

`PUT` is used to push a group of metrics. All metrics with the
grouping key specified in the URL are replaced by the metrics pushed
with `PUT`.

The body of the request contains the metrics to push either as delimited binary
protocol buffers or in the simple flat text format (both in version 0.0.4, see
the
[data exposition format specification](https://docs.google.com/document/d/1ZjyKiKxZV83VI9ZKAXRGKaUKK2BIWCT7oiGBKDBpjEY/edit?usp=sharing)).
Discrimination between the two variants is done via the `Content-Type`
header. (Use the value `application/vnd.google.protobuf;
proto=io.prometheus.client.MetricFamily; encoding=delimited` for protocol
buffers, otherwise the text format is tried as a fall-back.)

The response code upon success is either 200, 202, or 400. A 200 response
implies a successful push, either replacing an existing group of metrics or
creating a new one. A 400 response can happen if the request is malformed or if
the pushed metrics are inconsistent with metrics pushed to other groups or
collide with metrics of the Pushgateway itself. An explanation is returned in
the body of the response and logged on error level. A 202 can only occur if the
`--push.disable-consistency-check` flag is set. In this case, pushed metrics
are just queued and not checked for consistency. Inconsistencies will lead to
failed scrapes, however, as described [above](#about-metric-inconsistencies).

In rare cases, it is possible that the Pushgateway ends up with an inconsistent
set of metrics already pushed. In that case, new pushes are also rejected as
inconsistent even if the culprit is metrics that were pushed earlier. Delete
the offending metrics to get out of that situation.

_If using the protobuf format, do not send duplicate MetricFamily
proto messages (i.e. more than one with the same name) in one push, as
they will overwrite each other._

Note that the Pushgateway doesn't provide any strong guarantees that the pushed
metrics are persisted to disk. (A server crash may cause data loss. Or the
Pushgateway is configured to not persist to disk at all.)

A `PUT` request with an empty body effectively deletes all metrics with the
specified grouping key. However, in contrast to the
[`DELETE` request](#delete-method) described below, it does update the
`push_time_seconds` metrics.

### `POST` method

`POST` works exactly like the `PUT` method but only metrics with the
same name as the newly pushed metrics are replaced (among those with
the same grouping key).

A `POST` request with an empty body merely updates the `push_time_seconds`
metrics but does not change any of the previously pushed metrics.

### `DELETE` method

`DELETE` is used to delete metrics from the Pushgateway. The request
must not contain any content. All metrics with the grouping key
specified in the URL are deleted.

The response code upon success is always 202. The delete
request is merely queued at that moment. There is no guarantee that the
request will actually be executed or that the result will make it to
the persistence layer (e.g. in case of a server crash). However, the
order of `PUT`/`POST` and `DELETE` request is guaranteed, i.e. if you
have successfully sent a `DELETE` request and then send a `PUT`, it is
guaranteed that the `DELETE` will be processed first (and vice versa).

Deleting a grouping key without metrics is a no-op and will not result
in an error.

## Admin API

The Admin API provides administrative access to the Pushgateway, and must be
explicitly enabled by setting `--web.enable-admin-api` flag.

### URL

The default port the Pushgateway is listening to is 9091. The path looks like:

    /api/<API_VERSION>/admin/<HANDLER>
    
 * Available endpoints:
 
| HTTP_METHOD| API_VERSION |  HANDLER | DESCRIPTION |
| :-------: |:-------------:| :-----:| :----- |
| PUT     | v1 | wipe |  Safely deletes all metrics from the Pushgateway. |


* For example to wipe all metrics from the Pushgateway:

        curl -X PUT http://pushgateway.example.org:9091/api/v1/admin/wipe

## Query API

The query API allows accessing pushed metrics and build and runtime information.

### URL

    /api/<API_VERSION>/<HANDLER>
    
 * Available endpoints:
 
| HTTP_METHOD| API_VERSION |  HANDLER | DESCRIPTION |
| :-------: |:-------------:| :-----:| :----- |
| GET     | v1 | status |  Returns build information, command line flags, and the start time in JSON format. |
| GET     | v1 | metrics |  Returns the pushed metric families in JSON format. |


* For example :

        curl -X GET http://pushgateway.example.org:9091/api/v1/status | jq
        
        {
          "status": "success",
          "data": {
            "build_information": {
              "branch": "master",
              "buildDate": "20200310-20:14:39",
              "buildUser": "flipbyte@localhost.localdomain",
              "goVersion": "go1.13.6",
              "revision": "eba0ec4100873d23666bcf4b8b1d44617d6430c4",
              "version": "1.1.0"
            },
            "flags": {
              "log.format": "logfmt",
              "log.level": "info",
              "persistence.file": "",
              "persistence.interval": "5m0s",
              "push.disable-consistency-check": "false",
              "web.enable-admin-api": "false",
              "web.enable-lifecycle": "false",
              "web.external-url": "",
              "web.listen-address": ":9091",
              "web.route-prefix": "",
              "web.telemetry-path": "/metrics"
            },
            "start_time": "2020-03-11T01:44:49.9189758+05:30"
          }
        }
        
        curl -X GET http://pushgateway.example.org:9091/api/v1/metrics | jq
        
        {
          "status": "success",
          "data": [
            {
              "labels": {
                "job": "batch"
              },
              "last_push_successful": true,
              "my_job_duration_seconds": {
                "time_stamp": "2020-03-11T02:02:27.716605811+05:30",
                "type": "GAUGE",
                "help": "Duration of my batch jon in seconds",
                "metrics": [
                  {
                    "labels": {
                      "instance": "",
                      "job": "batch"
                    },
                    "value": "0.2721322309989773"
                  }
                ]
              },
              "push_failure_time_seconds": {
                "time_stamp": "2020-03-11T02:02:27.716605811+05:30",
                "type": "GAUGE",
                "help": "Last Unix time when changing this group in the Pushgateway failed.",
                "metrics": [
                  {
                    "labels": {
                      "instance": "",
                      "job": "batch"
                    },
                    "value": "0"
                  }
                ]
              },
              "push_time_seconds": {
                "time_stamp": "2020-03-11T02:02:27.716605811+05:30",
                "type": "GAUGE",
                "help": "Last Unix time when changing this group in the Pushgateway succeeded.",
                "metrics": [
                  {
                    "labels": {
                      "instance": "",
                      "job": "batch"
                    },
                    "value": "1.5838723477166057e+09"
                  }
                ]
              }
            }
          ]
        }
        
## Management API

The Pushgateway provides a set of management API to ease automation and integrations.

* Available endpoints:
 
| HTTP_METHOD |  PATH | DESCRIPTION |
| :-------: | :-----| :----- |
| GET    | /-/healthy |  Returns 200 whenever the Pushgateway is healthy. |
| GET    | /-/ready |  Returns 200 whenever the Pushgateway is ready to serve traffic. |

* The following endpoint is disabled by default and can be enabled via the `--web.enable-lifecycle` flag.

| HTTP_METHOD |  PATH | DESCRIPTION |
| :-------: | :-----| :----- |
| PUT    | /-/quit |  Triggers a graceful shutdown of Pushgateway. |

Alternatively, a graceful shutdown can be triggered by sending a `SIGTERM` to the Pushgateway process.

## Exposed metrics

The Pushgateway exposes the following metrics via the configured
`--web.telemetry-path` (default: `/metrics`):
- The pushed metrics.
- For each pushed group, a metric `push_time_seconds` and
  `push_failure_time_seconds` as explained above.
- The usual metrics provided by the [Prometheus Go client library](https://github.com/prometheus/client_golang), i.e.:
  - `process_...`
  - `go_...`
  - `promhttp_metric_handler_requests_...`
- A number of metrics specific to the Pushgateway, as documented by the example
  scrape below.

```
# HELP pushgateway_build_info A metric with a constant '1' value labeled by version, revision, branch, and goversion from which pushgateway was built.
# TYPE pushgateway_build_info gauge
pushgateway_build_info{branch="master",goversion="go1.10.2",revision="8f88ccb0343fc3382f6b93a9d258797dcb15f770",version="0.5.2"} 1
# HELP pushgateway_http_push_duration_seconds HTTP request duration for pushes to the Pushgateway.
# TYPE pushgateway_http_push_duration_seconds summary
pushgateway_http_push_duration_seconds{method="post",quantile="0.1"} 0.000116755
pushgateway_http_push_duration_seconds{method="post",quantile="0.5"} 0.000192608
pushgateway_http_push_duration_seconds{method="post",quantile="0.9"} 0.000327593
pushgateway_http_push_duration_seconds_sum{method="post"} 0.001622878
pushgateway_http_push_duration_seconds_count{method="post"} 8
# HELP pushgateway_http_push_size_bytes HTTP request size for pushes to the Pushgateway.
# TYPE pushgateway_http_push_size_bytes summary
pushgateway_http_push_size_bytes{method="post",quantile="0.1"} 166
pushgateway_http_push_size_bytes{method="post",quantile="0.5"} 182
pushgateway_http_push_size_bytes{method="post",quantile="0.9"} 196
pushgateway_http_push_size_bytes_sum{method="post"} 1450
pushgateway_http_push_size_bytes_count{method="post"} 8
# HELP pushgateway_http_requests_total Total HTTP requests processed by the Pushgateway, excluding scrapes.
# TYPE pushgateway_http_requests_total counter
pushgateway_http_requests_total{code="200",handler="static",method="get"} 5
pushgateway_http_requests_total{code="200",handler="status",method="get"} 8
pushgateway_http_requests_total{code="202",handler="delete",method="delete"} 1
pushgateway_http_requests_total{code="202",handler="push",method="post"} 6
pushgateway_http_requests_total{code="400",handler="push",method="post"} 2

```

### Alerting on failed pushes

It is in general a good idea to alert on `push_time_seconds` being much farther
behind than expected. This will catch both failed pushes as well as pushers
being down completely.

To detect failed pushes much earlier, alert on `push_failure_time_seconds >
push_time_seconds`.

Pushes can also fail because they are malformed. In this case, they never reach
any metric group and therefore won't set any `push_failure_time_seconds`
metrics. Those pushes are still counted as
`pushgateway_http_requests_total{code="400",handler="push"}`. You can alert on
the `rate` of this metric, but you have to inspect the logs to identify the
offending pusher.

## TLS and basic authentication

The Pushgateway supports TLS and basic authentication. This enables better
control of the various HTTP endpoints.

To use TLS and/or basic authentication, you need to pass a configuration file
using the `--web.config.file` parameter. The format of the file is described
[in the exporter-toolkit repository](https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md).

Note that the TLS and basic authentication settings affect all HTTP endpoints:
/metrics for scraping, the API to push metrics via /metrics/..., the admin API
via /api/..., and the web UI.

## Development

The normal binary embeds the web files in the `resources` directory.
For development purposes, it is handy to have a running binary use
those files directly (so that you can see the effect of changes immediately).
To switch to direct usage, add `-tags dev` to the `flags` entry in
`.promu.yml`, and then `make build`. Switch back to "normal" mode by
reverting the changes to `.promu.yml` and typing `make assets`.

##  Contributing

Relevant style guidelines are the [Go Code Review
Comments](https://code.google.com/p/go-wiki/wiki/CodeReviewComments)
and the _Formatting and style_ section of Peter Bourgon's [Go:
Best Practices for Production
Environments](http://peter.bourgon.org/go-in-production/#formatting-and-style).

[hub]: https://hub.docker.com/r/prom/pushgateway/
[circleci]: https://circleci.com/gh/prometheus/pushgateway
[quay]: https://quay.io/repository/prometheus/pushgateway
