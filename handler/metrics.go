// Copyright 2018 The Prometheus Authors
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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpCnt = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pushgateway_http_requests_total",
			Help: "Total HTTP requests processed by the Pushgateway, excluding scrapes.",
		},
		[]string{"handler", "code", "method"},
	)
	httpPushSize = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "pushgateway_http_push_size_bytes",
			Help:       "HTTP request size for pushes to the Pushgateway.",
			Objectives: map[float64]float64{0.1: 0.01, 0.5: 0.05, 0.9: 0.01},
		},
		[]string{"method"},
	)
	httpPushDuration = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "pushgateway_http_push_duration_seconds",
			Help:       "HTTP request duration for pushes to the Pushgateway.",
			Objectives: map[float64]float64{0.1: 0.01, 0.5: 0.05, 0.9: 0.01},
		},
		[]string{"method"},
	)
)

func InstrumentWithCounter(handlerName string, handler http.Handler) http.HandlerFunc {
	return promhttp.InstrumentHandlerCounter(
		httpCnt.MustCurryWith(prometheus.Labels{"handler": handlerName}),
		handler,
	)
}
