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

package handler

import (
	"net/http"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"

	"github.com/monzo/pushgateway/storage"
)

// Delete returns a handler that accepts delete requests.
//
// The returned handler is already instrumented for Prometheus.
func Delete(ms storage.MetricStore) func(http.ResponseWriter, *http.Request, httprouter.Params) {
	var ps httprouter.Params
	var mtx sync.Mutex // Protects ps.

	instrumentedHandlerFunc := prometheus.InstrumentHandlerFunc(
		"delete",
		func(w http.ResponseWriter, _ *http.Request) {
			job := ps.ByName("job")
			labelsString := ps.ByName("labels")
			mtx.Unlock()

			labels, err := splitLabels(labelsString)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				log.Debugf("Failed to parse URL: %v, %v", labelsString, err.Error())
				return
			}
			if job == "" {
				http.Error(w, "job name is required", http.StatusBadRequest)
				log.Debug("job name is required")
				return
			}
			labels["job"] = job
			ms.SubmitWriteRequest(storage.WriteRequest{
				Labels:    labels,
				Timestamp: time.Now(),
			})
			w.WriteHeader(http.StatusAccepted)
		},
	)
	return func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		mtx.Lock()
		ps = params
		instrumentedHandlerFunc(w, r)
	}
}

// LegacyDelete returns a handler that accepts delete requests. It deals with
// the deprecated API.
//
// The returned handler is already instrumented for Prometheus.
func LegacyDelete(ms storage.MetricStore) func(http.ResponseWriter, *http.Request, httprouter.Params) {
	var ps httprouter.Params
	var mtx sync.Mutex // Protects ps.

	instrumentedHandlerFunc := prometheus.InstrumentHandlerFunc(
		"delete",
		func(w http.ResponseWriter, _ *http.Request) {
			job := ps.ByName("job")
			instance := ps.ByName("instance")
			mtx.Unlock()

			if job == "" {
				http.Error(w, "job name is required", http.StatusBadRequest)
				log.Debug("job name is required")
				return
			}
			labels := map[string]string{"job": job}
			if instance != "" {
				labels["instance"] = instance
			}
			ms.SubmitWriteRequest(storage.WriteRequest{
				Labels:    labels,
				Timestamp: time.Now(),
			})
			w.WriteHeader(http.StatusAccepted)
		},
	)

	return func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		mtx.Lock()
		ps = params
		instrumentedHandlerFunc(w, r)
	}
}
