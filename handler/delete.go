package handler

import (
	"net/http"
	"time"

	"github.com/go-martini/martini"

	"github.com/prometheus/pushgateway/storage"
)

// Delete returns a handler that accepts delete requests. If only a job is
// specified in the query, all metrics for that job are deleted. If a job and an
// instance is specified, all metrics for that job/instance combination are
// deleted.
func Delete(ms storage.MetricStore) func(martini.Params, http.ResponseWriter) {
	return func(params martini.Params, w http.ResponseWriter) {
		job := params["job"]
		if job == "" {
			http.Error(w, "job name is required", http.StatusBadRequest)
			return
		}
		instance := params["instance"]
		ms.SubmitWriteRequest(storage.WriteRequest{
			Job:       job,
			Instance:  instance,
			Timestamp: time.Now(),
		})
		w.WriteHeader(http.StatusAccepted)
	}
}
