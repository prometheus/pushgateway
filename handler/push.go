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
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/julienschmidt/httprouter"
	"github.com/matttproud/golang_protobuf_extensions/pbutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	dto "github.com/prometheus/client_model/go"

	"github.com/monzo/pushgateway/storage"
)

const (
	pushMetricName = "push_time_seconds"
	pushMetricHelp = "Last Unix time when this group was changed in the Pushgateway."
)

// Push returns an http.Handler which accepts samples over HTTP and stores them
// in the MetricStore. If replace is true, all metrics for the job and instance
// given by the request are deleted before new ones are stored.
//
// The returned handler is already instrumented for Prometheus.
func Push(
	ms storage.MetricStore, replace bool,
) func(http.ResponseWriter, *http.Request, httprouter.Params) {
	var ps httprouter.Params
	var mtx sync.Mutex // Protects ps.

	instrumentedHandlerFunc := prometheus.InstrumentHandlerFunc(
		"push",
		func(w http.ResponseWriter, r *http.Request) {
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

			if replace {
				ms.SubmitWriteRequest(storage.WriteRequest{
					Labels:    labels,
					Timestamp: time.Now(),
				})
			}

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
				log.Debugf("Failed to parse text, %v", err.Error())
				return
			}
			if timestampsPresent(metricFamilies) {
				http.Error(w, "pushed metrics must not have timestamps", http.StatusBadRequest)
				log.Debug("pushed metrics must not have timestamps")
				return
			}
			now := time.Now()
			addPushTimestamp(metricFamilies, now)
			sanitizeLabels(metricFamilies, labels)
			ms.SubmitWriteRequest(storage.WriteRequest{
				Labels:         labels,
				Timestamp:      now,
				MetricFamilies: metricFamilies,
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

// LegacyPush returns an http.Handler which accepts samples over HTTP and stores
// them in the MetricStore. It uses the deprecated API (expecting a 'job'
// parameter and an optional 'instance' parameter). If replace is true, all
// metrics for the job and instance given by the request are deleted before new
// ones are stored.
//
// The returned handler is already instrumented for Prometheus.
func LegacyPush(
	ms storage.MetricStore, replace bool,
) func(http.ResponseWriter, *http.Request, httprouter.Params) {
	var ps httprouter.Params
	var mtx sync.Mutex // Protects ps.

	instrumentedHandlerFunc := prometheus.InstrumentHandlerFunc(
		"push",
		func(w http.ResponseWriter, r *http.Request) {
			job := ps.ByName("job")
			instance := ps.ByName("instance")
			mtx.Unlock()

			var err error
			if job == "" {
				http.Error(w, "job name is required", http.StatusBadRequest)
				log.Debug("job name is required")
				return
			}
			if instance == "" {
				// Remote IP number (without port).
				instance, _, err = net.SplitHostPort(r.RemoteAddr)
				if err != nil || instance == "" {
					instance = "localhost"
				}
			}
			labels := map[string]string{"job": job, "instance": instance}
			if replace {
				ms.SubmitWriteRequest(storage.WriteRequest{
					Labels:    labels,
					Timestamp: time.Now(),
				})
			}

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
				http.Error(w, err.Error(), http.StatusInternalServerError)
				log.Debugf("Error parsing request body, %v", err.Error())
				return
			}
			if timestampsPresent(metricFamilies) {
				http.Error(w, "pushed metrics must not have timestamps", http.StatusBadRequest)
				log.Debug("pushed metrics must not have timestamps")
				return
			}
			now := time.Now()
			addPushTimestamp(metricFamilies, now)
			sanitizeLabels(metricFamilies, labels)
			ms.SubmitWriteRequest(storage.WriteRequest{
				Labels:         labels,
				Timestamp:      now,
				MetricFamilies: metricFamilies,
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

// sanitizeLabels ensures that all the labels in groupingLabels and the
// `instance` label are present in each MetricFamily in metricFamilies. The
// label values from groupingLabels are set in each MetricFamily, no matter
// what. After that, if the 'instance' label is not present at all in a
// MetricFamily, it will be created (with an empty string as value).
//
// Finally, sanitizeLabels sorts the label pairs of all metrics.
func sanitizeLabels(
	metricFamilies map[string]*dto.MetricFamily,
	groupingLabels map[string]string,
) {
	gLabelsNotYetDone := make(map[string]string, len(groupingLabels))

	for _, mf := range metricFamilies {
	metric:
		for _, m := range mf.GetMetric() {
			for ln, lv := range groupingLabels {
				gLabelsNotYetDone[ln] = lv
			}
			hasInstanceLabel := false
			for _, lp := range m.GetLabel() {
				ln := lp.GetName()
				if lv, ok := gLabelsNotYetDone[ln]; ok {
					lp.Value = proto.String(lv)
					delete(gLabelsNotYetDone, ln)
				}
				if ln == string(model.InstanceLabel) {
					hasInstanceLabel = true
				}
				if len(gLabelsNotYetDone) == 0 && hasInstanceLabel {
					sort.Sort(prometheus.LabelPairSorter(m.Label))
					continue metric
				}
			}
			for ln, lv := range gLabelsNotYetDone {
				m.Label = append(m.Label, &dto.LabelPair{
					Name:  proto.String(ln),
					Value: proto.String(lv),
				})
				if ln == string(model.InstanceLabel) {
					hasInstanceLabel = true
				}
				delete(gLabelsNotYetDone, ln) // To prepare map for next metric.
			}
			if !hasInstanceLabel {
				m.Label = append(m.Label, &dto.LabelPair{
					Name:  proto.String(string(model.InstanceLabel)),
					Value: proto.String(""),
				})
			}
			sort.Sort(prometheus.LabelPairSorter(m.Label))
		}
	}
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
		if !model.LabelNameRE.MatchString(components[i]) ||
			strings.HasPrefix(components[i], model.ReservedLabelPrefix) {
			return nil, fmt.Errorf("improper label name %q", components[i])
		}
		result[components[i]] = components[i+1]
	}
	return result, nil
}

// Checks if any timestamps have been specified.
func timestampsPresent(metricFamilies map[string]*dto.MetricFamily) bool {
	for _, mf := range metricFamilies {
		for _, m := range mf.GetMetric() {
			if m.TimestampMs != nil {
				return true
			}
		}
	}
	return false
}

// Add metric to indicate the push time.
func addPushTimestamp(metricFamilies map[string]*dto.MetricFamily, t time.Time) {
	metricFamilies[pushMetricName] = &dto.MetricFamily{
		Name: proto.String(pushMetricName),
		Help: proto.String(pushMetricHelp),
		Type: dto.MetricType_GAUGE.Enum(),
		Metric: []*dto.Metric{
			{
				Gauge: &dto.Gauge{
					Value: proto.Float64(float64(t.UnixNano()) / 1e9),
				},
			},
		},
	}
}
