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

package handler

import (
	"net/http"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/pushgateway/storage"
)

// Delete returns a handler that accepts delete requests. If only a job is
// specified in the query, all metrics for that job are deleted. If a job and an
// instance is specified, all metrics for that job/instance combination are
// deleted.
//
// The returned handler is already instrumented for Prometheus.
func Delete(ms storage.MetricStore) func(http.ResponseWriter, *http.Request, httprouter.Params) {
	var ps httprouter.Params

	instrumentedHandlerFunc := prometheus.InstrumentHandlerFunc(
		"delete",
		func(w http.ResponseWriter, _ *http.Request) {
			job := ps.ByName("job")
			if job == "" {
				http.Error(w, "job name is required", http.StatusBadRequest)
				return
			}
			instance := ps.ByName("instance")
			ms.SubmitWriteRequest(storage.WriteRequest{
				Job:       job,
				Instance:  instance,
				Timestamp: time.Now(),
			})
			w.WriteHeader(http.StatusAccepted)
		},
	)

	return func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		ps = params
		instrumentedHandlerFunc(w, r)
	}
}
