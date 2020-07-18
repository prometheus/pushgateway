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
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/matttproud/golang_protobuf_extensions/pbutil"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/route"

	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/pushgateway/storage"
)

const (
	// Base64Suffix is appended to a label name in the request URL path to
	// mark the following label value as base64 encoded.
	Base64Suffix = "@base64"
)

// Push returns an http.Handler which accepts samples over HTTP and stores them
// in the MetricStore. If replace is true, all metrics for the job and instance
// given by the request are deleted before new ones are stored. If check is
// true, the pushed metrics are immediately checked for consistency (with
// existing metrics and themselves), and an inconsistent push is rejected with
// http.StatusBadRequest.
//
// The returned handler is already instrumented for Prometheus.
func Push(
	ms storage.MetricStore,
	replace, check, jobBase64Encoded bool,
	logger log.Logger,
) func(http.ResponseWriter, *http.Request) {
	var mtx sync.Mutex // Protects ps.

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		job := route.Param(r.Context(), "job")
		if jobBase64Encoded {
			var err error
			if job, err = decodeBase64(job); err != nil {
				http.Error(w, fmt.Sprintf("invalid base64 encoding in job name %q: %v", job, err), http.StatusBadRequest)
				level.Debug(logger).Log("msg", "invalid base64 encoding in job name", "job", job, "err", err.Error())
				return
			}
		}
		labelsString := route.Param(r.Context(), "labels")
		mtx.Unlock()

		labels, err := splitLabels(labelsString)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			level.Debug(logger).Log("msg", "failed to parse URL", "url", labelsString, "err", err.Error())
			return
		}
		if job == "" {
			http.Error(w, "job name is required", http.StatusBadRequest)
			level.Debug(logger).Log("msg", "job name is required")
			return
		}
		labels["job"] = job

		var metricFamilies map[string]*dto.MetricFamily
		ctMediatype, ctParams, ctErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if ctErr == nil && ctMediatype == "application/vnd.google.protobuf" &&
			ctParams["encoding"] == "delimited" &&
			ctParams["proto"] == "io.prometheus.client.MetricFamily" {
			metricFamilies = map[string]*dto.MetricFamily{}
			for {
				mf := &dto.MetricFamily{}
				if _, err = pbutil.ReadDelimited(r.Body, mf); err != nil {
					if err == io.EOF {
						err = nil
					}
					break
				}
				metricFamilies[mf.GetName()] = mf
			}
		} else {
			// We could do further content-type checks here, but the
			// fallback for now will anyway be the text format
			// version 0.0.4, so just go for it and see if it works.
			var parser expfmt.TextParser
			metricFamilies, err = parser.TextToMetricFamilies(r.Body)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			level.Debug(logger).Log("msg", "failed to parse text", "source", r.RemoteAddr, "err", err.Error())
			return
		}
		now := time.Now()
		if !check {
			ms.SubmitWriteRequest(storage.WriteRequest{
				Labels:         labels,
				Timestamp:      now,
				MetricFamilies: metricFamilies,
				Replace:        replace,
			})
			w.WriteHeader(http.StatusAccepted)
			return
		}
		errCh := make(chan error, 1)
		errReceived := false
		ms.SubmitWriteRequest(storage.WriteRequest{
			Labels:         labels,
			Timestamp:      now,
			MetricFamilies: metricFamilies,
			Replace:        replace,
			Done:           errCh,
		})
		for err := range errCh {
			// Send only first error via HTTP, but log all of them.
			// TODO(beorn): Consider sending all errors once we
			// have a use case. (Currently, at most one error is
			// produced.)
			if !errReceived {
				http.Error(
					w,
					fmt.Sprintf("pushed metrics are invalid or inconsistent with existing metrics: %v", err),
					http.StatusBadRequest,
				)
			}
			level.Error(logger).Log(
				"msg", "pushed metrics are invalid or inconsistent with existing metrics",
				"method", r.Method,
				"source", r.RemoteAddr,
				"err", err.Error(),
			)
			errReceived = true
		}
	})

	instrumentedHandler := promhttp.InstrumentHandlerRequestSize(
		httpPushSize, promhttp.InstrumentHandlerDuration(
			httpPushDuration, InstrumentWithCounter("push", handler),
		))

	return func(w http.ResponseWriter, r *http.Request) {
		mtx.Lock()
		instrumentedHandler.ServeHTTP(w, r)
	}
}

// decodeBase64 decodes the provided string using the “Base 64 Encoding with URL
// and Filename Safe Alphabet” (RFC 4648). Padding characters (i.e. trailing
// '=') are ignored.
func decodeBase64(s string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
	return string(b), err
}

// splitLabels splits a labels string into a label map mapping names to values.
func splitLabels(labels string) (map[string]string, error) {
	result := map[string]string{}
	if len(labels) <= 1 {
		return result, nil
	}
	components := strings.Split(labels[1:], "/")
	if len(components)%2 != 0 {
		return nil, fmt.Errorf("odd number of components in label string %q", labels)
	}

	for i := 0; i < len(components)-1; i += 2 {
		name, value := components[i], components[i+1]
		trimmedName := strings.TrimSuffix(name, Base64Suffix)
		if !model.LabelNameRE.MatchString(trimmedName) ||
			strings.HasPrefix(trimmedName, model.ReservedLabelPrefix) {
			return nil, fmt.Errorf("improper label name %q", trimmedName)
		}
		if name == trimmedName {
			result[name] = value
			continue
		}
		decodedValue, err := decodeBase64(value)
		if err != nil {
			return nil, fmt.Errorf("invalid base64 encoding for label %s=%q: %v", trimmedName, value, err)
		}
		result[trimmedName] = decodedValue
	}
	return result, nil
}
