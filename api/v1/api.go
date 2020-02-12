package v1

import (
	"encoding/json"
	// "errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	// "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/route"
	"github.com/prometheus/prometheus/pkg/textparse"
	// "github.com/prometheus/prometheus/util/httputil"
	
)

type status string

const (
	statusSuccess status = "success"
	statusError status = "error"
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
	ready    func(http.HandlerFunc) http.HandlerFunc
	uptime   time.Time
	logger   log.Logger
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
) *API {
	if l == nil {
		l = log.NewNopLogger()
	}

	return &API{
		uptime:         time.Now(),
		logger:         l,
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

	// r.Options("/*path", wrap(api.options))

	// r.Get("/metadata",wrap(api.metricMetadata))
	r.Get("/check",wrap(api.check))
}

type metadata struct {
	Type textparse.MetricType `json:"type"`
	Help string               `json:"help"`
	Unit string               `json:"unit"`
}

// func (api *API) options(r *http.Request) apiFuncResult {
// 	return apiFuncResult{nil, nil, nil}
// }

func (api *API) check(w http.ResponseWriter, r *http.Request){
	res := make(map[string]string)
	res["message"] = "hello world!!!"

	api.respond(w,res)
}

// TODO: Make changes in this file for displaying metadata
// func (api *API) metricMetadata(r *http.Request) apiFuncResult {
// 	metrics := map[string]map[metadata]struct{}{}

// 	limit := -1
// 	if s := r.FormValue("limit"); s != "" {
// 		var err error
// 		if limit, err = strconv.Atoi(s); err != nil {
// 			return apiFuncResult{nil, &apiError{errorBadData, errors.New("limit must be a number")}, nil, nil}
// 		}
// 	}

// 	metric := r.FormValue("metric")

// 	for _, tt := range api.targetRetriever.TargetsActive() {
// 		for _, t := range tt {

// 			if metric == "" {
// 				for _, mm := range t.MetadataList() {
// 					m := metadata{Type: mm.Type, Help: mm.Help, Unit: mm.Unit}
// 					ms, ok := metrics[mm.Metric]

// 					if !ok {
// 						ms = map[metadata]struct{}{}
// 						metrics[mm.Metric] = ms
// 					}
// 					ms[m] = struct{}{}
// 				}
// 				continue
// 			}

// 			if md, ok := t.Metadata(metric); ok {
// 				m := metadata{Type: md.Type, Help: md.Help, Unit: md.Unit}
// 				ms, ok := metrics[md.Metric]

// 				if !ok {
// 					ms = map[metadata]struct{}{}
// 					metrics[md.Metric] = ms
// 				}
// 				ms[m] = struct{}{}
// 			}
// 		}
// 	}

// 	// Put the elements from the pseudo-set into a slice for marshaling.
// 	res := map[string][]metadata{}

// 	for name, set := range metrics {
// 		if limit >= 0 && len(res) >= limit {
// 			break
// 		}

// 		s := []metadata{}
// 		for metadata := range set {
// 			s = append(s, metadata)
// 		}
// 		res[name] = s
// 	}

// 	return apiFuncResult{res,nil,nil,nil}
// }

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