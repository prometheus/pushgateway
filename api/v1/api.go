package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/route"

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
	ready       func(http.HandlerFunc) http.HandlerFunc
	uptime      time.Time
	logger      log.Logger
	MetricStore storage.MetricStore
	Flags       map[string]string
	BuildTime   time.Time
	BuildInfo   map[string]string
}

type apiFuncResult struct {
	data      interface{}
	err       *apiError
	finalizer func()
}

type apiFunc func(r *http.Request) apiFuncResult

// New returns a new API.
func New(
	l log.Logger,
	ms storage.MetricStore,
	f map[string]string,
	build time.Time,
	buildInfo map[string]string,
) *API {
	if l == nil {
		l = log.NewNopLogger()
	}

	return &API{
		uptime:      time.Now(),
		logger:      l,
		MetricStore: ms,
		Flags:       f,
		BuildTime:   build,
		BuildInfo:   buildInfo,
	}
}

// Register registers the API handlers under their correct routes
// in the given router.
func (api *API) Register(r *route.Router) {
	wrap := func(f http.HandlerFunc) http.HandlerFunc {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			setCORS(w)
			f(w, r)
		})
	}

	r.Options("/*path", wrap(func(w http.ResponseWriter, r *http.Request) {}))

	r.Get("/metadata", wrap(api.metricMetadata))
	r.Get("/status", wrap(api.status))
}

type metadata struct {
	Type string `json:"type"`
	Help string `json:"help"`
}

func (api *API) metricMetadata(w http.ResponseWriter, r *http.Request) {
	familyMaps := api.MetricStore.GetMetricFamiliesMap()
	res := make(map[string]interface{})
	for n, v := range familyMaps {
		metricResponse := make(map[string]interface{})
		metricResponse["label"] = v.Labels
		for _, metricValues := range v.Metrics {
			uniqueMetrics := [1]metadata{
				{
					Type: metricValues.GobbableMetricFamily.Type.String(),
					Help: *metricValues.GobbableMetricFamily.Help,
				},
			}
			metricResponse[*metricValues.GobbableMetricFamily.Name] = uniqueMetrics
		}
		res[n] = metricResponse
	}

	api.respond(w, res)
}

func (api *API) status(w http.ResponseWriter, r *http.Request) {
	res := make(map[string]interface{})
	res["flags"] = api.Flags
	res["buildTime"] = api.BuildTime
	res["buildInformation"] = api.BuildInfo

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
		level.Error(api.logger).Log("msg", "Error marshaling JSON", "err", err)
		return
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
