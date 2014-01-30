package main

import (
	"expvar"
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/bmizerany/pat"
)

func main() {
	var (
		addr = flag.String("addr", ":8080", "address to listen on")

		ttl              = flag.Duration("ttl", 30*time.Minute, "how long to cache received metrics")
		evictionInterval = flag.Duration("evictionInterval", 5*time.Second, "how often to check for expired metrics")

		registry = newRegistry()
		cache    = newCache()
		mux      = pat.New()
	)

	flag.Parse()

	registry.Publish(runtimeMetrics)
	registry.Publish(cache)
	expvar.Publish("cache", cache)

	mux.Get("/metrics", registry)
	mux.Put("/metrics/job/:job/instance/:instance", pushHandler(cache, *ttl))

	http.Handle("/", mux)

	go cache.Evict(time.Tick(*evictionInterval))

	log.Println("listening on", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
