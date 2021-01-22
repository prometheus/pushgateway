## 1.4.0 / 2021-01-23

* [FEATURE] **Experimental!** Add TLS and basic authentication to HTTP endpoints. #381

## 1.3.1 / 2020-12-17

* [ENHANCEMENT] Web UI: Improved metrics text alignment. #369
* [BUGFIX] Web UI: Fix deletion of groups with empty label values. #377

## 1.3.0 / 2020-10-01

* [FEATURE] Add Docker image build for ppc64le architecture. #339
* [ENHANCEMENT] Web UI: Add scroll bare to list of pushed metrics. #354
* [ENHANCEMENT] Logging: Show remote address when failing to parse pushed metrics. #361
* [BUGFIX] Web UI: Update JQuery to v3.5.1 to address security concerns. #360

## 1.2.0 / 2020-03-11

* [FEATURE] Add an HTTP API to query pushed metrics and runtime information. #184

## 1.1.0 / 2020-01-27

* [FEATURE] Add flag `--push.disable-consistency-check`. #318

## 1.0.1 / 2019-12-21

* [ENHANCEMENT] Remove excessive whitespace from HTML templates. #302
* [BUGFIX] Fix docker manifest files for non-amd64 architectures. #310

## 1.0.0 / 2019-10-15

_This release does not support the storage format of v0.5–v0.9 anymore. Only persistence files created by v0.10+ are usable. Upgrade to v0.10 first to convert existing persistence files._

* [CHANGE] Remove code to convert the legacy v0.5–v0.9 storage format.

## 0.10.0 / 2019-10-10

_This release changes the storage format. v0.10 can read the storage format of v0.5–v0.9. It will then persist the new format so that a downgrade won't be possible anymore._

* [CHANGE] Change of the storage format (necessary for the hash collision bugfix below). #293
* [CHANGE] Check pushed metrics immediately and reject them if inconsistent. Successful pushes now result in code 200 (not 202). Failures result in code 400 and are logged at error level. #290
* [FEATURE] Shutdown via HTTP request. Enable with `--web.enable-lifecycle`. #292
* [FEATURE] Wipe storage completely via HTTP request and via web UI. Enable with `--web.enable-admin-api`. #287 #285
* [BUGFIX] Rule out hash collisions between metric groups. #293
* [BUGFIX] Avoid multiple calls of `http.Error` in push handler. #291

## 0.9.1 / 2019-08-01

* [BUGFIX] Make `--web.external-url` and `--web.route-prefix` work as documented. #274

## 0.9.0 / 2019-07-23

* [CHANGE] Web: Update to Bootstrap 4.3.1 and jquery 3.4.1, changing appearance of the web UI to be more in line with the Prometheus server. Also add favicon and remove timestamp column. #261
* [CHANGE] Update logging to be in line with other Prometheus projects, using gokit and promlog. #263
* [FEATURE] Add optional base64 encoding for label values in the grouping key. #268
* [FEATURE] Add ARM container images. #265
* [FEATURE] Log errors during scrapes. #267
* [BUGFIX] Web: Fixed Content-Type for js and css instead of using /etc/mime.types. #252

## 0.8.0 / 2019-04-13

_If you use the prebuilt Docker container or you build your own one based on the provided Dockerfile, note that this release changes the user to `nobody`. Should you use a persistence file, make sure it is readable and writable by user `nobody`._

* [CHANGE] Run as user `nobody` in Docker. #242
* [CHANGE] Adjust `--web.route-prefix` to work the same as in Prometheus. #190
* [FEATURE] Add `--web.external-url` flag (like in Prometheus). #190

## 0.7.0 / 2018-12-07

_As preparation for the 1.0.0 release, this release removes the long deprecated legacy HTTP push endpoint (which uses `/jobs/` rather than `/job/` in the URL)._

* [CHANGE] Remove legacy push API. #227
* [ENHANCEMENT] Update dependencies. #230
* [ENHANCEMENT] Support Go modules. #221
* [BUGFIX] Avoid crash when started with v0.4 storage. #223

## 0.6.0 / 2018-10-17

_Persistence storage prior to 0.5.0 is unsupported. Upgrade to 0.5.2 first for conversion._

* [CHANGE] Enforce consistency of help strings by changing them during exposition. (An INFO-level log message describes the change.) #194
* [CHANGE] Drop support of pre-0.5 storage format.
* [CHANGE] Use prometheus/client_golang v0.9, which changes the `http_...` metrics. (See README.md for full documentation of exposed metrics.)

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
* [FEATURE] When being scraped, metrics of the same name but with different job/instance label are now merged into one metric family.
* [FEATURE] Added Dockerfile.
* [CHANGE] Default HTTP port now 9091.
* [BUGFIX] Fixed parsing of content-type header.	
* [BUGFIX] Fixed race condition in handlers.
* [PERFORMANCE] Replaced Martini with Httprouter.
* [ENHANCEMENT] Migrated to new client_golang.
* [ENHANCEMENT]	Made internal metrics more consistent.
* [ENHANCEMENT]	Added http instrumentation.

