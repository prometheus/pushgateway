// Copyright 2019 The Prometheus Authors
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

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	"github.com/prometheus/pushgateway/storage"
)

// WipeMetricStore deletes all the metrics in MetricStore.
//
// The returned handler is already instrumented for Prometheus.
func WipeMetricStore(
	ms storage.MetricStore,
	logger log.Logger) http.Handler {

	return InstrumentWithCounter(
		"wipe",
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			level.Debug(logger).Log("msg", "start wiping metric store")
			// Delete all metric groups by sending write requests with MetricFamilies equal to nil.
			for _, group := range ms.GetMetricFamiliesMap() {
				ms.SubmitWriteRequest(storage.WriteRequest{
					Labels:    group.Labels,
					Timestamp: time.Now(),
				})
			}

		}))
}
