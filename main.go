// Copyright 2014 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/elazarl/go-bindata-assetfs"
	"github.com/julienschmidt/httprouter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"

	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/pushgateway/handler"
	"github.com/prometheus/pushgateway/storage"
)

func init() {
	prometheus.MustRegister(version.NewCollector("pushgateway"))
}

func main() {
	var (
		app = kingpin.New(filepath.Base(os.Args[0]), "The Pushgateway")

		listenAddress       = app.Flag("web.listen-address", "Address to listen on for the web interface, API, and telemetry.").Default(":9091").String()
		metricsPath         = app.Flag("web.telemetry-path", "Path under which to expose metrics.").Default("/metrics").String()
		routePrefix         = app.Flag("web.route-prefix", "Prefix for the internal routes of web endpoints.").Default("").String()
		persistenceFile     = app.Flag("persistence.file", "File to persist metrics. If empty, metrics are only kept in memory.").Default("").String()
		persistenceInterval = app.Flag("persistence.interval", "The minimum interval at which to write out the persistence file.").Default("5m").Duration()
	)
	log.AddFlags(app)
	app.Version(version.Print("pushgateway"))
	app.HelpFlag.Short('h')
	kingpin.MustParse(app.Parse(os.Args[1:]))

	if *routePrefix == "/" {
		*routePrefix = ""
	}
	if *routePrefix != "" {
		*routePrefix = "/" + strings.Trim(*routePrefix, "/")
	}

	log.Infoln("Starting pushgateway", version.Info())
	log.Infoln("Build context", version.BuildContext())
	log.Debugf("Prefix path is '%s'", *routePrefix)

	flags := map[string]string{}
	for _, f := range app.Model().Flags {
		flags[f.Name] = f.Value.String()
	}
	ms := storage.NewDiskMetricStore(*persistenceFile, *persistenceInterval)

	// Inject the metric families returned by ms.GetMetricFamilies into the default Gatherer:
	prometheus.DefaultGatherer = prometheus.Gatherers{
		prometheus.DefaultGatherer,
		prometheus.GathererFunc(func() ([]*dto.MetricFamily, error) { return ms.GetMetricFamilies(), nil }),
	}

	r := httprouter.New()
	r.Handler("GET", *routePrefix+"/-/healthy", prometheus.InstrumentHandlerFunc("healthy", handler.Healthy(ms)))
	r.Handler("GET", *routePrefix+"/-/ready", prometheus.InstrumentHandlerFunc("ready", handler.Ready(ms)))

	r.Handler("GET", path.Join(*routePrefix, *metricsPath), prometheus.Handler())

	// Handlers for pushing and deleting metrics.
	pushAPIPath := *routePrefix + "/metrics"
	r.PUT(pushAPIPath+"/job/:job/*labels", handler.Push(ms, true))
	r.POST(pushAPIPath+"/job/:job/*labels", handler.Push(ms, false))
	r.DELETE(pushAPIPath+"/job/:job/*labels", handler.Delete(ms))
	r.PUT(pushAPIPath+"/job/:job", handler.Push(ms, true))
	r.POST(pushAPIPath+"/job/:job", handler.Push(ms, false))
	r.DELETE(pushAPIPath+"/job/:job", handler.Delete(ms))

	// Handlers for the deprecated API.
	r.PUT(pushAPIPath+"/jobs/:job/instances/:instance", handler.LegacyPush(ms, true))
	r.POST(pushAPIPath+"/jobs/:job/instances/:instance", handler.LegacyPush(ms, false))
	r.DELETE(pushAPIPath+"/jobs/:job/instances/:instance", handler.LegacyDelete(ms))
	r.PUT(pushAPIPath+"/jobs/:job", handler.LegacyPush(ms, true))
	r.POST(pushAPIPath+"/jobs/:job", handler.LegacyPush(ms, false))
	r.DELETE(pushAPIPath+"/jobs/:job", handler.LegacyDelete(ms))

	r.Handler("GET", *routePrefix+"/static/*filepath", prometheus.InstrumentHandler(
		"static",
		http.FileServer(
			&assetfs.AssetFS{
				Asset: func(name string) ([]byte, error) {
					path := name[len(*routePrefix):]
					return Asset(path)
				},
				AssetDir: AssetDir, AssetInfo: AssetInfo},
		),
	))
	statusHandler := prometheus.InstrumentHandlerFunc("status", handler.Status(ms, Asset, flags))
	r.Handler("GET", *routePrefix+"/status", statusHandler)
	r.Handler("GET", *routePrefix+"/", statusHandler)

	// Re-enable pprof.
	r.GET(*routePrefix+"/debug/pprof/*pprof", handlePprof)

	log.Infof("Listening on %s.", *listenAddress)
	l, err := net.Listen("tcp", *listenAddress)
	if err != nil {
		log.Fatal(err)
	}
	go interruptHandler(l)
	err = (&http.Server{Addr: *listenAddress, Handler: r}).Serve(l)
	log.Errorln("HTTP server stopped:", err)
	// To give running connections a chance to submit their payload, we wait
	// for 1sec, but we don't want to wait long (e.g. until all connections
	// are done) to not delay the shutdown.
	time.Sleep(time.Second)
	if err := ms.Shutdown(); err != nil {
		log.Errorln("Problem shutting down metric storage:", err)
	}
}

func handlePprof(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	switch p.ByName("pprof") {
	case "/cmdline":
		pprof.Cmdline(w, r)
	case "/profile":
		pprof.Profile(w, r)
	case "/symbol":
		pprof.Symbol(w, r)
	default:
		pprof.Index(w, r)
	}
}

func interruptHandler(l net.Listener) {
	notifier := make(chan os.Signal, 1)
	signal.Notify(notifier, os.Interrupt, syscall.SIGTERM)
	<-notifier
	log.Info("Received SIGINT/SIGTERM; exiting gracefully...")
	l.Close()
}
