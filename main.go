// Copyright 2014 Prometheus Team
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
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/pushgateway/handler"
	"github.com/prometheus/pushgateway/storage"
)

var (
	addr                = flag.String("addr", ":9091", "Address to listen on.")
	persistenceFile     = flag.String("persistence.file", "", "File to persist metrics. If empty, metrics are only kept in memory.")
	persistenceInterval = flag.Duration("persistence.interval", 5*time.Minute, "The minimum interval at which to write out the persistence file.")

	im = internalMetrics{
		{
			desc: prometheus.NewDesc(
				"runtime_goroutines_count",
				"Number of goroutines that currently exist.",
				nil, nil,
			),
			eval:    func(ms *runtime.MemStats) float64 { return float64(runtime.NumGoroutine()) },
			valType: prometheus.GaugeValue,
			// Not a counter, despite the name... It can go up and down.
		},
		{
			desc: prometheus.NewDesc(
				"runtime_memory_allocated_bytes",
				"Bytes allocated and still in use.",
				nil, nil,
			),
			eval:    func(ms *runtime.MemStats) float64 { return float64(ms.Alloc) },
			valType: prometheus.GaugeValue,
		},
		{
			desc: prometheus.NewDesc(
				"runtime_memory_allocated_bytes_total",
				"Total bytes ever allocated (even if freed).",
				nil, nil,
			),
			eval:    func(ms *runtime.MemStats) float64 { return float64(ms.TotalAlloc) },
			valType: prometheus.CounterValue,
		},
		{
			desc: prometheus.NewDesc(
				"runtime_memory_heap_allocated_bytes",
				"Heap bytes allocated and still in use.",
				nil, nil,
			),
			eval:    func(ms *runtime.MemStats) float64 { return float64(ms.HeapAlloc) },
			valType: prometheus.GaugeValue,
		},
		{
			desc: prometheus.NewDesc(
				"runtime_gc_high_watermark_bytes",
				"Next run in HeapAlloc time (bytes).",
				nil, nil,
			),
			eval:    func(ms *runtime.MemStats) float64 { return float64(ms.NextGC) },
			valType: prometheus.GaugeValue,
		},
		{
			desc: prometheus.NewDesc(
				"runtime_gc_pause_ns",
				"Total GC pause time.",
				nil, nil,
			),
			eval:    func(ms *runtime.MemStats) float64 { return float64(ms.PauseTotalNs) },
			valType: prometheus.GaugeValue,
		},
		{
			desc: prometheus.NewDesc(
				"runtime_gc_total",
				"Total number of GC runs.",
				nil, nil,
			),
			eval:    func(ms *runtime.MemStats) float64 { return float64(ms.NumGC) },
			valType: prometheus.CounterValue,
		},
	}
)

type internalMetrics []struct {
	desc    *prometheus.Desc
	eval    func(*runtime.MemStats) float64
	valType prometheus.ValueType
}

func (im internalMetrics) Describe(ch chan<- *prometheus.Desc) {
	for _, i := range im {
		ch <- i.desc
	}
}

func (im internalMetrics) Collect(ch chan<- prometheus.Metric) {
	memStats := &runtime.MemStats{}
	runtime.ReadMemStats(memStats)
	for _, i := range im {
		ch <- prometheus.MustNewConstMetric(i.desc, i.valType, i.eval(memStats))
	}
}

func main() {
	flag.Parse()
	versionInfoTmpl.Execute(os.Stdout, BuildInfo)
	flags := map[string]string{}
	flag.VisitAll(func(f *flag.Flag) {
		flags[f.Name] = f.Value.String()
	})

	ms := storage.NewDiskMetricStore(*persistenceFile, *persistenceInterval)
	prometheus.SetMetricFamilyInjectionHook(ms.GetMetricFamilies)

	prometheus.MustRegister(im)

	r := httprouter.New()
	r.Handler("GET", "/metrics", prometheus.Handler())
	r.PUT("/metrics/jobs/:job/instances/:instance", handler.Push(ms, true))
	r.POST("/metrics/jobs/:job/instances/:instance", handler.Push(ms, false))
	r.DELETE("/metrics/jobs/:job/instances/:instance", handler.Delete(ms))
	r.PUT("/metrics/jobs/:job", handler.Push(ms, true))
	r.POST("/metrics/jobs/:job", handler.Push(ms, false))
	r.DELETE("/metrics/jobs/:job", handler.Delete(ms))
	r.Handler("GET", "/functions.js", prometheus.InstrumentHandlerFunc(
		"static",
		func(w http.ResponseWriter, _ *http.Request) {
			if b, err := Asset("resources/functions.js"); err == nil {
				w.Write(b)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		},
	))
	statusHandler := prometheus.InstrumentHandlerFunc("status", handler.Status(ms, Asset, flags, BuildInfo))
	r.Handler("GET", "/status", statusHandler)
	r.Handler("GET", "/", statusHandler)

	// Re-enable pprof.
	r.GET("/debug/pprof/*pprof", HandlePprof)

	log.Printf("Listening on %s.\n", *addr)
	l, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	go interruptHandler(l)
	err = (&http.Server{Addr: *addr, Handler: r}).Serve(l)
	log.Print("HTTP server stopped: ", err)
	// To give running connections a chance to submit their payload, we wait
	// for 1sec, but we don't want to wait long (e.g. until all connections
	// are done) to not delay the shutdown.
	time.Sleep(time.Second)
	if err := ms.Shutdown(); err != nil {
		log.Print("Problem shutting down metric storage: ", err)
	}
}

func HandlePprof(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
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
