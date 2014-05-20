package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/go-martini/martini"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/pushgateway/handler"
	"github.com/prometheus/pushgateway/storage"
)

var (
	addr                = flag.String("addr", ":8080", "Address to listen on.")
	persistenceFile     = flag.String("persistence.file", "", "File to persist metrics. If empty, metrics are only kept in memory.")
	persistenceInterval = flag.Duration("persistence.interval", 5*time.Minute, "The minimum interval at which to write out the persistence file.")

	internalMetrics = []*struct {
		name   string
		help   string
		eval   func(*runtime.MemStats) float64
		metric prometheus.Metric
	}{
		{
			name: "instance_goroutine_count",
			help: "The number of goroutines that currently exist.",
			eval: func(ms *runtime.MemStats) float64 {
				return float64(runtime.NumGoroutine())
			},
			metric: prometheus.NewGauge(),
			// Not a counter, despite the name... It can go up and down.
		},
		{
			name:   "instance_allocated_bytes",
			help:   "Bytes allocated and still in use.",
			eval:   func(ms *runtime.MemStats) float64 { return float64(ms.Alloc) },
			metric: prometheus.NewGauge(),
		},
		{
			name:   "instance_total_allocated_bytes",
			help:   "Bytes allocated (even if freed).",
			eval:   func(ms *runtime.MemStats) float64 { return float64(ms.TotalAlloc) },
			metric: prometheus.NewGauge(),
		},
		{
			name:   "instance_heap_allocated_bytes",
			help:   "Heap bytes allocated and still in use.",
			eval:   func(ms *runtime.MemStats) float64 { return float64(ms.HeapAlloc) },
			metric: prometheus.NewGauge(),
		},
		{
			name:   "instance_gc_high_watermark_bytes",
			help:   "Next run in HeapAlloc time (bytes).",
			eval:   func(ms *runtime.MemStats) float64 { return float64(ms.NextGC) },
			metric: prometheus.NewGauge(),
		},
		{
			name:   "instance_gc_total_pause_ns",
			help:   "Total GC pause time.",
			eval:   func(ms *runtime.MemStats) float64 { return float64(ms.PauseTotalNs) },
			metric: prometheus.NewGauge(),
		},
		{
			name:   "instance_gc_count",
			help:   "GC count.",
			eval:   func(ms *runtime.MemStats) float64 { return float64(ms.NumGC) },
			metric: prometheus.NewCounter(),
		},
	}
)

func main() {
	flag.Parse()
	versionInfoTmpl.Execute(os.Stdout, BuildInfo)
	flags := map[string]string{}
	flag.VisitAll(func(f *flag.Flag) {
		flags[f.Name] = f.Value.String()
	})
	m := martini.Classic()

	ms := storage.NewDiskMetricStore(*persistenceFile, *persistenceInterval)
	prometheus.DefaultRegistry.SetMetricFamilyInjectionHook(ms.GetMetricFamilies)

	// The following demonstrate clearly the clunkiness of the current Go
	// client library when it comes to values that are owned by other parts
	// of the program and have to be evaluated on the fly. Let's leave it
	// here as a demonstration and a benchmark for improvements of the
	// library.
	// TODO(bjoern): Update this once the new client library is out.
	registerInternalMetrics()
	m.Get("/metrics", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			updateInternalMetrics()
			prometheus.DefaultHandler(w, r)
		}))

	m.Put("/metrics/jobs/:job/instances/:instance", handler.Push(ms))
	m.Post("/metrics/jobs/:job/instances/:instance", handler.Push(ms))
	m.Delete("/metrics/jobs/:job/instances/:instance", handler.Delete(ms))
	m.Put("/metrics/jobs/:job", handler.Push(ms))
	m.Post("/metrics/jobs/:job", handler.Push(ms))
	m.Delete("/metrics/jobs/:job", handler.Delete(ms))
	m.Get("/functions.js", func() ([]byte, error) { return Asset("resources/functions.js") })
	statusHandler := handler.Status(ms, Asset, flags, BuildInfo)
	m.Get("/status", statusHandler)
	m.Get("/", statusHandler)

	http.Handle("/", m)

	log.Printf("Listening on %s.\n", *addr)
	l, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	go interruptHandler(l)
	err = (&http.Server{Addr: *addr}).Serve(l)
	log.Print("HTTP server stopped: ", err)
	// To give running connections a chance to submit their payload, we wait
	// for 1sec, but we don't want to wait long (e.g. until all connections
	// are done) to not delay the shutdown.
	time.Sleep(time.Second)
	if err := ms.Shutdown(); err != nil {
		log.Print("Problem shutting down metric storage: ", err)
	}
}

func interruptHandler(l net.Listener) {
	notifier := make(chan os.Signal)
	signal.Notify(notifier, os.Interrupt, syscall.SIGTERM)
	<-notifier
	log.Print("Received SIGINT/SIGTERM; exiting gracefully...")
	l.Close()
}

func registerInternalMetrics() {
	for _, im := range internalMetrics {
		prometheus.Register(im.name, im.help, nil, im.metric)
	}
}

func updateInternalMetrics() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	for _, im := range internalMetrics {
		switch m := im.metric.(type) {
		case prometheus.Gauge:
			m.Set(nil, im.eval(&memStats))
		case prometheus.Counter:
			m.Set(nil, im.eval(&memStats))
		default:
			log.Print("Unexpected metric type: ", m)
		}
	}
}
