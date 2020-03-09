// Copyright 2020 The Prometheus Authors
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
package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/route"

	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/pushgateway/handler"
	"github.com/prometheus/pushgateway/storage"
)

type status string

const (
	statusSuccess status = "success"
	statusError   status = "error"
)

type errorType string

const (
	errorNone        errorType = ""
	errorTimeout     errorType = "timeout"
	errorCanceled    errorType = "canceled"
	errorExec        errorType = "execution"
	errorBadData     errorType = "bad_data"
	errorInternal    errorType = "internal"
	errorUnavailable errorType = "unavailable"
	errorNotFound    errorType = "not_found"
)

type apiError struct {
	typ errorType
	err error
}

func (e *apiError) Error() string {
	return fmt.Sprintf("%s: %s", e.typ, e.err)
}

var corsHeaders = map[string]string{
	"Access-Control-Allow-Headers":  "Accept, Authorization, Content-Type, Origin",
	"Access-Control-Allow-Methods":  "GET, POST, DELETE, OPTIONS",
	"Access-Control-Allow-Origin":   "*",
	"Access-Control-Expose-Headers": "Date",
	"Cache-Control":                 "no-cache, no-store, must-revalidate",
}

// Enables cross-site script calls.
func setCORS(w http.ResponseWriter) {
	for h, v := range corsHeaders {
		w.Header().Set(h, v)
	}
}

// API provides registration of handlers for API routes.
type API struct {
	logger      log.Logger
	MetricStore storage.MetricStore
	Flags       map[string]string
	StartTime   time.Time
	BuildInfo   map[string]string
}

// New returns a new API. The log.Logger can be nil, in which case no logging is performed.
func New(
	l log.Logger,
	ms storage.MetricStore,
	flags map[string]string,
	buildInfo map[string]string,
) *API {
	if l == nil {
		l = log.NewNopLogger()
	}

	return &API{
		StartTime:   time.Now(),
		logger:      l,
		MetricStore: ms,
		Flags:       flags,
		BuildInfo:   buildInfo,
	}
}

// Register registers the API handlers under their correct routes
// in the given router.
func (api *API) Register(r *route.Router) {
	wrap := func(handlerName string, f http.HandlerFunc) http.HandlerFunc {
		return handler.InstrumentWithCounter(
			handlerName,
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				setCORS(w)
				f(w, r)
			}),
		)
	}

	r.Options("/*path", wrap("api/v1/options", func(w http.ResponseWriter, r *http.Request) {}))

	r.Get("/status", wrap("api/v1/status", api.status))
	r.Get("/metrics", wrap("api/v1/metrics", api.metrics))
}

type metrics struct {
	Timestamp time.Time         `json:"time_stamp"`
	Type      string            `json:"type"`
	Help      string            `json:"help,omitempty"`
	Metrics   []encodableMetric `json:"metrics"`
}

func (api *API) metrics(w http.ResponseWriter, r *http.Request) {
	familyMaps := api.MetricStore.GetMetricFamiliesMap()
	res := []interface{}{}
	for _, v := range familyMaps {
		metricResponse := map[string]interface{}{}
		metricResponse["labels"] = v.Labels
		metricResponse["last_push_successful"] = v.LastPushSuccess()
		for name, metricValues := range v.Metrics {
			metricFamily := metricValues.GetMetricFamily()
			uniqueMetrics := metrics{
				Type:      metricFamily.GetType().String(),
				Help:      metricFamily.GetHelp(),
				Timestamp: metricValues.Timestamp,
				Metrics:   makeEncodableMetrics(metricFamily.GetMetric(), metricFamily.GetType()),
			}
			metricResponse[name] = uniqueMetrics
		}
		res = append(res, metricResponse)
	}

	api.respond(w, res)
}

func (api *API) status(w http.ResponseWriter, r *http.Request) {
	res := map[string]interface{}{}
	res["flags"] = api.Flags
	res["start_time"] = api.StartTime
	res["build_information"] = api.BuildInfo

	api.respond(w, res)
}

type response struct {
	Status    status      `json:"status"`
	Data      interface{} `json:"data,omitempty"`
	ErrorType errorType   `json:"errorType,omitempty"`
	Error     string      `json:"error,omitempty"`
}

func (api *API) respond(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)

	b, err := json.Marshal(&response{
		Status: statusSuccess,
		Data:   data,
	})
	if err != nil {
		level.Error(api.logger).Log("msg", "error marshaling JSON", "err", err)
		api.respondError(w, apiError{
			typ: errorBadData,
			err: err,
		}, "")
	}

	if _, err := w.Write(b); err != nil {
		level.Error(api.logger).Log("msg", "failed to write data to connection", "err", err)
	}
}

func (api *API) respondError(w http.ResponseWriter, apiErr apiError, data interface{}) {
	w.Header().Set("Content-Type", "application/json")

	switch apiErr.typ {
	case errorBadData:
		w.WriteHeader(http.StatusBadRequest)
	case errorInternal:
		w.WriteHeader(http.StatusInternalServerError)
	default:
		panic(fmt.Sprintf("unknown error type %q", apiErr.Error()))
	}

	b, err := json.Marshal(&response{
		Status:    statusError,
		ErrorType: apiErr.typ,
		Error:     apiErr.err.Error(),
		Data:      data,
	})
	if err != nil {
		return
	}
	level.Error(api.logger).Log("msg", "API error", "err", apiErr.Error())

	if _, err := w.Write(b); err != nil {
		level.Error(api.logger).Log("msg", "failed to write data to connection", "err", err)
	}
}

type encodableMetric map[string]interface{}

func makeEncodableMetrics(metrics []*dto.Metric, metricsType dto.MetricType) []encodableMetric {

	jsonMetrics := make([]encodableMetric, len(metrics))

	for i, m := range metrics {

		metric := encodableMetric{}
		metric["labels"] = makeLabels(m)
		switch metricsType {
		case dto.MetricType_SUMMARY:
			metric["quantiles"] = makeQuantiles(m)
			metric["count"] = fmt.Sprint(m.GetSummary().GetSampleCount())
			metric["sum"] = fmt.Sprint(m.GetSummary().GetSampleSum())
		case dto.MetricType_HISTOGRAM:
			metric["buckets"] = makeBuckets(m)
			metric["count"] = fmt.Sprint(m.GetHistogram().GetSampleCount())
			metric["sum"] = fmt.Sprint(m.GetHistogram().GetSampleSum())
		default:
			metric["value"] = fmt.Sprint(getValue(m))
		}
		jsonMetrics[i] = metric
	}
	return jsonMetrics
}

func makeLabels(m *dto.Metric) map[string]string {
	result := map[string]string{}
	for _, lp := range m.Label {
		result[lp.GetName()] = lp.GetValue()
	}
	return result
}

func makeQuantiles(m *dto.Metric) map[string]string {
	result := map[string]string{}
	for _, q := range m.GetSummary().Quantile {
		result[fmt.Sprint(q.GetQuantile())] = fmt.Sprint(q.GetValue())
	}
	return result
}

func makeBuckets(m *dto.Metric) map[string]string {
	result := map[string]string{}
	for _, b := range m.GetHistogram().Bucket {
		result[fmt.Sprint(b.GetUpperBound())] = fmt.Sprint(b.GetCumulativeCount())
	}
	return result
}

func getValue(m *dto.Metric) float64 {
	switch {
	case m.Gauge != nil:
		return m.GetGauge().GetValue()
	case m.Counter != nil:
		return m.GetCounter().GetValue()
	case m.Untyped != nil:
		return m.GetUntyped().GetValue()
	default:
		return 0
	}
}
