## 0.5.2 / 2018-06-15

* [BUGFIX] Update client_golang/prometheus vendoring to allow inconsistent
  labels. #185

## 0.5.1 / 2018-05-30

* [BUGFIX] Fix conversion of old persistency format (0.4.0 and earlier). #179
* [BUGFIX] Make _Delete Group_ button work again. #177
* [BUGFIX] Don't display useless flags on status page. #176

## 0.5.0 / 2018-05-23

Breaking change:
* Flags now require double-dash.
* The persistence storage format has been updated.  Upgrade is transparent, but downgrade to 0.4.0 and prior is unsupported.
* Persistence storage prior to 0.1.0 is unsupported.

* [CHANGE] Replaced Flags with Kingpin #152
* [CHANGE] Slightly changed disk format for persistence. v0.5 can still read the pre-v0.5 format. #172
* [ENHANCEMENT] Debug level logging now shows client-induced errors #123
* [FEATURE] Add /-/ready and /-/healthy #135
* [FEATURE] Add web.route-prefix flag #146
* [BUGFIX] Fix incorrect persistence of certain values in a metric family. #172

## 0.4.0 / 2017-06-09
* [CHANGE] Pushes with timestamps are now rejected.
* [FEATURE] Added push_time_seconds metric to each push.
* [ENHANCEMENT] Point at community page rather than the dev list in the UI.
* [BUGFIX] Return HTTP 400 on parse error, rather than 500.

## 0.3.1 / 2016-11-03
* [BUGFIX] Fixed a race condition in the storage layer.
* [ENHANCEMENT] Improved README.md.

## 0.3.0 / 2016-06-07
* [CHANGE] Push now rejects improper and reserved labels.
* [CHANGE] Required labels flag removed.
* [BUGFIX] Docker image actually works now.
* [ENHANCEMENT] Converted to Promu build process.
* [CHANGE] As a consequence of the above, changed dir structure in tar ball.
* [ENHANCEMENT] Updated dependencies, with all the necessary code changes.
* [ENHANCEMENT] Dependencies now vendored.
* [ENHANCEMENT] `bindata.go` checked in, Pushgateway now `go get`-able.
* [ENHANCEMENT] Various documentation improvements.
* [CLEANUP] Various code cleanups.

## 0.2.0 / 2015-06-25
* [CHANGE] Support arbitrary grouping of metrics.
* [CHANGE] Changed behavior of HTTP DELETE method (see README.md for details).

## 0.1.2 / 2015-06-08
* [CHANGE] Move pushgateway binary in archive from bin/ to /.
* [CHANGE] Migrate logging to prometheus/log.

## 0.1.1 / 2015-05-05
* [BUGFIX] Properly display histograms in web status.
* [BUGFIX] Fix value formatting.
* [CHANGE] Make flag names consistent across projects.
* [ENHANCEMENT] Auto-fill instance with IPv6 address.
* [BUGFIX] Fix Go download link for several archs and OSes.
* [BUGFIX] Use HTTPS and golang.org for Go download.
* [BUGFIX] Re-add pprof endpoints.

## 0.1.0 / 2014-08-13
* [FEATURE] When being scraped, metrics of the same name but with different
  job/instance label are now merged into one metric family.
* [FEATURE] Added Dockerfile.
* [CHANGE] Default HTTP port now 9091.
* [BUGFIX] Fixed parsing of content-type header.	
* [BUGFIX] Fixed race condition in handlers.
* [PERFORMANCE] Replaced Martini with Httprouter.
* [ENHANCEMENT] Migrated to new client_golang.
* [ENHANCEMENT]	Made internal metrics more consistent.
* [ENHANCEMENT]	Added http instrumentation.

