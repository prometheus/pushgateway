# prometheus exporter

A prometheus exporter exists to allow ephemeral and batch jobs to expose their
metrics to Prometheus. Since these kinds of jobs may not exist long enough to
be scraped, they can instead push their metrics to the exporter. The exporter
then exposes these metrics to prometheus.

It is explicitly not an aggreagator, but rather a metrics cache; that is, it
expects to receive a complete set of metrics (once, or periodically) to export.

### cli

> coming soon...

### api

#### `PUT /metrics/job/<job_name>/instance/<instance_name>`

For example, a job which runs periodically on a host and captures system stats
it might submit its metrics to `/metrics/job/system_stats/instance/host-0001`.
The job and instance names will be exposed to prometheus as additional labels
for the metrics.

The body of the request should be the binary encoded protobuf format described
[here](https://github.com/prometheus/client_model/blob/feature/new-proto-format/prometheus.proto#L25).
