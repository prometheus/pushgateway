package main

import (
	"io"
	"net/http"
	"time"
)

const sampleContentType = `application/vnd.google.protobuf;proto=io.prometheus.client.Sample;encoding=delimited`

// pushHandler returns an http.Handler which accepts samples over HTTP and
// stores them in cache.
func pushHandler(cache *cache, ttl time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		if r.Header.Get("Content-Type") != sampleContentType {
			http.Error(w, "", http.StatusUnsupportedMediaType)
			return
		}

		var (
			query = r.URL.Query()

			jobName      = query.Get(":job")
			instanceName = query.Get(":instance")

			err error

			now     = time.Now()
			metrics = Metrics{Expires: now.Add(ttl)}

			dec = newDecoder(r.Body)
		)

		if jobName == "" || instanceName == "" {
			http.Error(w, "job and instance names are required", http.StatusBadRequest)
			return
		}

		for {
			var sample = Sample{Timestamp: now.Unix()}

			if err := dec.Decode(&sample); err != nil {
				if err == io.EOF {
					err = nil
				}

				break
			}

			metrics.Samples = append(metrics.Samples, sample)
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		cache.Set(jobName, instanceName, metrics)

		w.WriteHeader(http.StatusNoContent)
	})
}
