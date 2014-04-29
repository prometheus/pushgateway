package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bmizerany/pat"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/pushgateway/handler"
	"github.com/prometheus/pushgateway/storage"
)

var (
	addr                = flag.String("addr", ":8080", "Address to listen on.")
	persistenceFile     = flag.String("persistence.file", "", "File to persist metrics. If empty, metrics are only kept in memory.")
	persistenceDuration = flag.Duration("persistence.duration", 5*time.Minute, "Do not write the persistence file more often than that.")
)

func main() {
	flag.Parse()
	mux := pat.New()

	ms := storage.NewDiskMetricStore(*persistenceFile, *persistenceDuration)

	prometheus.DefaultRegistry.SetMetricFamilyInjectionHook(ms.GetMetricFamilies)
	// TODO: expose some internal metrics

	mux.Get("/metrics", prometheus.DefaultHandler)
	mux.Put("/metrics/job/:job/instance/:instance", handler.Push(ms))
	mux.Post("/metrics/job/:job/instance/:instance", handler.Push(ms))
	mux.Del("/metrics/job/:job/instance/:instance", handler.Delete(ms))
	mux.Put("/metrics/job/:job", handler.Push(ms))
	mux.Post("/metrics/job/:job", handler.Push(ms))
	mux.Del("/metrics/job/:job", handler.Delete(ms))
	// TODO: Add web interface

	http.Handle("/", mux)

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
