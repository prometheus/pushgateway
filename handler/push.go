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
	"io"
	"mime"
	"net"
	"net/http"
	"sync"
	"time"

	"code.google.com/p/goprotobuf/proto"
	"github.com/julienschmidt/httprouter"
	"github.com/matttproud/golang_protobuf_extensions/ext"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/text"

	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/pushgateway/storage"
)

// Push returns an http.Handler which accepts samples over HTTP and stores them
// in the MetricStore. If replace is true, all metrics for the job and instance
// given by the request are deleted before new ones are stored.
//
// The returned handler is already instrumented for Prometheus.
func Push(ms storage.MetricStore, replace bool) func(http.ResponseWriter, *http.Request, httprouter.Params) {
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
				return
			}
			if instance == "" {
				// Remote IP number (without port).
				instance, _, err = net.SplitHostPort(r.RemoteAddr)
				if err != nil || instance == "" {
					instance = "localhost"
				}
			}
			if replace {
				ms.SubmitWriteRequest(storage.WriteRequest{
					Job:       job,
					Instance:  instance,
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
					if _, err = ext.ReadDelimited(r.Body, mf); err != nil {
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
				var parser text.Parser
				metricFamilies, err = parser.TextToMetricFamilies(r.Body)
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			setJobAndInstance(metricFamilies, job, instance)
			ms.SubmitWriteRequest(storage.WriteRequest{
				Job:            job,
				Instance:       instance,
				Timestamp:      time.Now(),
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

func setJobAndInstance(metricFamilies map[string]*dto.MetricFamily, job, instance string) {
	for _, mf := range metricFamilies {
	metric:
		for _, m := range mf.GetMetric() {
			var jobDone, instanceDone bool
			for _, lp := range m.GetLabel() {
				switch lp.GetName() {
				case "job":
					lp.Value = proto.String(job)
					jobDone = true
				case "instance":
					lp.Value = proto.String(instance)
					instanceDone = true
				}
				if jobDone && instanceDone {
					continue metric
				}
			}
			if !jobDone {
				m.Label = append(m.Label, &dto.LabelPair{
					Name:  proto.String("job"),
					Value: proto.String(job),
				})
			}
			if !instanceDone {
				m.Label = append(m.Label, &dto.LabelPair{
					Name:  proto.String("instance"),
					Value: proto.String(instance),
				})
			}
		}
	}
}
