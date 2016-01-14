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
	"flag"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elazarl/go-bindata-assetfs"
	"github.com/julienschmidt/httprouter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/log"

	"github.com/prometheus/pushgateway/handler"
	"github.com/prometheus/pushgateway/storage"
)

var (
	listenAddress       = flag.String("web.listen-address", ":9091", "Address to listen on for the web interface, API, and telemetry.")
	metricsPath         = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
	persistenceFile     = flag.String("persistence.file", "", "File to persist metrics. If empty, metrics are only kept in memory.")
	persistenceInterval = flag.Duration("persistence.interval", 5*time.Minute, "The minimum interval at which to write out the persistence file.")
)

func main() {
	flag.Parse()
	versionInfoTmpl.Execute(os.Stdout, BuildInfo)
	flags := map[string]string{}
	flag.VisitAll(func(f *flag.Flag) {
		flags[f.Name] = f.Value.String()
	})

	ms := storage.NewDiskMetricStore(*persistenceFile, *persistenceInterval)
	prometheus.SetMetricFamilyInjectionHook(ms.GetMetricFamilies)
	// Enable collect checks for debugging.
	// prometheus.EnableCollectChecks(true)

	r := httprouter.New()
	r.Handler("GET", *metricsPath, prometheus.Handler())

	// Handlers for pushing and deleting metrics.
	r.PUT("/metrics/job/:job/*labels", handler.Push(ms, true))
	r.POST("/metrics/job/:job/*labels", handler.Push(ms, false))
	r.DELETE("/metrics/job/:job/*labels", handler.Delete(ms))
	r.PUT("/metrics/job/:job", handler.Push(ms, true))
	r.POST("/metrics/job/:job", handler.Push(ms, false))
	r.DELETE("/metrics/job/:job", handler.Delete(ms))

	// Handlers for the deprecated API.
	r.PUT("/metrics/jobs/:job/instances/:instance", handler.LegacyPush(ms, true))
	r.POST("/metrics/jobs/:job/instances/:instance", handler.LegacyPush(ms, false))
	r.DELETE("/metrics/jobs/:job/instances/:instance", handler.LegacyDelete(ms))
	r.PUT("/metrics/jobs/:job", handler.LegacyPush(ms, true))
	r.POST("/metrics/jobs/:job", handler.LegacyPush(ms, false))
	r.DELETE("/metrics/jobs/:job", handler.LegacyDelete(ms))

	r.Handler("GET", "/static/*filepath", prometheus.InstrumentHandler(
		"static",
		http.FileServer(
			&assetfs.AssetFS{Asset: Asset, AssetDir: AssetDir, AssetInfo: AssetInfo},
		),
	))
	statusHandler := prometheus.InstrumentHandlerFunc("status", handler.Status(ms, Asset, flags, BuildInfo))
	r.Handler("GET", "/status", statusHandler)
	r.Handler("GET", "/", statusHandler)

	// Re-enable pprof.
	r.GET("/debug/pprof/*pprof", handlePprof)

	log.Printf("Listening on %s.", *listenAddress)
	l, err := net.Listen("tcp", *listenAddress)
	if err != nil {
		log.Fatal(err)
	}
	go interruptHandler(l)
	err = (&http.Server{Addr: *listenAddress, Handler: r}).Serve(l)
	log.Print("HTTP server stopped: ", err)
	// To give running connections a chance to submit their payload, we wait
	// for 1sec, but we don't want to wait long (e.g. until all connections
	// are done) to not delay the shutdown.
	time.Sleep(time.Second)
	if err := ms.Shutdown(); err != nil {
		log.Print("Problem shutting down metric storage: ", err)
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
	notifier := make(chan os.Signal)
	signal.Notify(notifier, os.Interrupt, syscall.SIGTERM)
	<-notifier
	log.Print("Received SIGINT/SIGTERM; exiting gracefully...")
	l.Close()
}
