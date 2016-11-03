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

