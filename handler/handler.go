package handler

import (
	"io"
	"net/http"
	"strings"
	"time"
	"code.google.com/p/goprotobuf/proto"

	"github.com/matttproud/golang_protobuf_extensions/ext"
	"github.com/prometheus/client_golang/text"

	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/pushgateway/storage"
)

const protobufContentType = `application/vnd.google.protobuf;proto=io.prometheus.client.Sample;encoding=delimited`

// Push returns an http.Handler which accepts samples over HTTP and
// stores them in cache.
func Push(ms storage.MetricStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		query := r.URL.Query()
		job := query.Get(":job")
		if job == "" {
			http.Error(w, "job name is required", http.StatusBadRequest)
			return
		}

		instance := query.Get(":instance")
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
	})
}

func Delete(ms storage.MetricStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		query := r.URL.Query()
		job := query.Get(":job")
		if job == "" {
			http.Error(w, "job name is required", http.StatusBadRequest)
			return
		}

		instance := query.Get(":instance")
		ms.SubmitWriteRequest(storage.WriteRequest{
			Job:       job,
			Instance:  instance,
			Timestamp: time.Now(),
		})
		w.WriteHeader(http.StatusAccepted)
	})
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
