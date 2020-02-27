// Copyright 2017 The Prometheus Authors
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
	"io"
	"net/http"

	"github.com/prometheus/common/server"

	"github.com/prometheus/pushgateway/storage"
)

// Healthy is used to report the health of the Pushgateway. It currently only
// uses the Healthy method of the MetricScore to detect healthy state.
//
// The returned handler is already instrumented for Prometheus.
func Healthy(ms storage.MetricStore) http.Handler {
	return InstrumentWithCounter(
		"healthy",
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			err := ms.Healthy()
			if err == nil {
				io.WriteString(w, "OK")
			} else {
				http.Error(w, err.Error(), 500)
			}
		}),
	)
}

// Ready is used to report if the Pushgateway is ready to process requests. It
// currently only uses the Ready method of the MetricScore to detect ready
// state.
//
// The returned handler is already instrumented for Prometheus.
func Ready(ms storage.MetricStore) http.Handler {
	return InstrumentWithCounter(
		"ready",
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			err := ms.Ready()
			if err == nil {
				io.WriteString(w, "OK")
			} else {
				http.Error(w, err.Error(), 500)
			}
		}),
	)
}

// Static serves the static files from the provided http.FileSystem.
//
// The returned handler is already instrumented for Prometheus.
func Static(root http.FileSystem, prefix string) http.Handler {
	if prefix == "/" {
		prefix = ""
	}

	handler := server.StaticFileServer(root)
	return InstrumentWithCounter(
		"static",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = r.URL.Path[len(prefix):]
			handler.ServeHTTP(w, r)
		}),
	)
}
