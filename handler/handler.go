package handler

import (
	"io"
	"net/http"
	"strings"
	"time"
	"code.google.com/p/goprotobuf/proto"

	"github.com/go-martini/martini"
	"github.com/matttproud/golang_protobuf_extensions/ext"
	"github.com/prometheus/client_golang/text"

	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/pushgateway/storage"
)

const protobufContentType = `application/vnd.google.protobuf;proto=io.prometheus.client.Sample;encoding=delimited`

// Push returns an http.Handler which accepts samples over HTTP and
// stores them in the MetricStore.
func Push(ms storage.MetricStore) func(martini.Params, http.ResponseWriter, *http.Request) {
	return func(params martini.Params, w http.ResponseWriter, r *http.Request) {
		job := params["job"]
		if job == "" {
			http.Error(w, "job name is required", http.StatusBadRequest)
			return
		}

		instance := params["instance"]
		if instance == "" {
			// Remote IP number (without port).
			instance = strings.SplitN(r.RemoteAddr, ":", 2)[0]
			if instance == "" {
				instance = "localhost"
			}
		}
		var err error
		var metricFamilies map[string]*dto.MetricFamily
		if r.Header.Get("Content-Type") == protobufContentType {
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
	}
}

// Delete returns an http.Handler which accepts delete requests. If only a job
// is specified in the query, all metrics for that job are deleted. If a job and
// an instance is specified, all metrics for that job/instance combination are
// deleted.
func Delete(ms storage.MetricStore) func(martini.Params, http.ResponseWriter) {
	return func(params martini.Params, w http.ResponseWriter) {
		job := params["job"]
		if job == "" {
			http.Error(w, "job name is required", http.StatusBadRequest)
			return
		}
		instance := params["instance"]
		ms.SubmitWriteRequest(storage.WriteRequest{
			Job:       job,
			Instance:  instance,
			Timestamp: time.Now(),
		})
		w.WriteHeader(http.StatusAccepted)
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
