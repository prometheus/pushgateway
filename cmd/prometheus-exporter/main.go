package main

import (
	"expvar"
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"

	"github.com/bmizerany/pat"
)

func main() {
	var (
		addr = flag.String("addr", ":8080", "address to listen on")

		registry = newRegistry()
		cache    = newCache()
		mux      = pat.New()
	)

	flag.Parse()

	registry.Publish(runtimeMetrics)
	registry.Publish(cache)
	expvar.Publish("cache", cache)

	mux.Get("/metrics", registry)
	mux.Put("/metrics/job/:job/instance/:instance", pushHandler(cache))

	http.Handle("/", mux)

	log.Println("listening on", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
