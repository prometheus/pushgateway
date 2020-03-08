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
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/go-kit/kit/log"
	"github.com/golang/protobuf/proto"
	"github.com/prometheus/pushgateway/storage"
)

var (
	logger    = log.NewNopLogger()
	testFlags = map[string]string{
		"flag1": "value1",
		"flag2": "value2",
		"flag3": "value3",
	}
	testBuildInfo = map[string]string{
		"build1": "value1",
		"build2": "value2",
		"build3": "value3",
	}

	mf1 = &dto.MetricFamily{
		Name: proto.String("mf1"),
		Type: dto.MetricType_UNTYPED.Enum(),
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("instance"),
						Value: proto.String("instance2"),
					},
					{
						Name:  proto.String("job"),
						Value: proto.String("job1"),
					},
				},
				Untyped: &dto.Untyped{
					Value: proto.Float64(-3e3),
				},
			},
		},
	}

	mf2 = &dto.MetricFamily{
		Name: proto.String("mf2"),
		Help: proto.String("doc string 2"),
		Type: dto.MetricType_GAUGE.Enum(),
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("basename"),
						Value: proto.String("basevalue2"),
					},
					{
						Name:  proto.String("instance"),
						Value: proto.String("instance2"),
					},
					{
						Name:  proto.String("job"),
						Value: proto.String("job1"),
					},
					{
						Name:  proto.String("labelname"),
						Value: proto.String("val2"),
					},
				},
				Gauge: &dto.Gauge{
					Value: proto.Float64(math.Inf(+1)),
				},
			},
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("instance"),
						Value: proto.String("instance2"),
					},
					{
						Name:  proto.String("job"),
						Value: proto.String("job1"),
					},
					{
						Name:  proto.String("labelname"),
						Value: proto.String("val1"),
					},
				},
				Gauge: &dto.Gauge{
					Value: proto.Float64(math.Inf(-1)),
				},
			},
		},
	}

	grouping1 = map[string]string{
		"job":      "job1",
		"instance": "instance1",
	}
)

func templateMetrics(metricStore storage.MetricStore) []interface{} {
	familyMaps := metricStore.GetMetricFamiliesMap()
	res := make([]interface{}, 0)
	for _, v := range familyMaps {
		metricResponse := make(map[string]interface{})
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
	return res
}

func compareMaps(mainMap map[string]interface{}, testMap map[string]string) bool {
	if len(mainMap) != len(testMap) {
		return false
	}

	for key, _ := range mainMap {
		if mainMap[key].(string) != testMap[key] {
			return false
		}
	}
	return true
}

func compareArray(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

func TestStatusAPI(t *testing.T) {
	dms := storage.NewDiskMetricStore("", 100*time.Millisecond, nil, logger)
	TestApi := New(logger, dms, testFlags, testBuildInfo)

	req, err := http.NewRequest("GET", "http://example.org/", &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()

	testResponse := response{}
	TestApi.status(w, req)
	json.Unmarshal(w.Body.Bytes(), &testResponse)
	jsonData := testResponse.Data.(map[string]interface{})
	responseFlagData := jsonData["flags"].(map[string]interface{})
	responseBuildInfo := jsonData["build_information"].(map[string]interface{})

	if expected, got := http.StatusOK, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}

	if !compareMaps(responseFlagData, testFlags) {
		t.Errorf("Wanted following flags %v, got %v.", testFlags, responseFlagData)
	}

	if !compareMaps(responseBuildInfo, testBuildInfo) {
		t.Errorf("Wanted following build info %v, got %v.", testBuildInfo, responseBuildInfo)
	}
}

func TestMetricsAPI(t *testing.T) {
	dms := storage.NewDiskMetricStore("", 100*time.Millisecond, nil, logger)
	TestApi := New(logger, dms, testFlags, testBuildInfo)

	req, err := http.NewRequest("GET", "http://example.org/", &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()

	TestApi.metrics(w, req)

	if expected, got := http.StatusOK, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}

	requiredResponse, _ := json.Marshal(&response{
		Status: statusSuccess,
		Data:   []interface{}{},
	})

	if !compareArray(requiredResponse, w.Body.Bytes()) {
		t.Errorf("Wanted response %v, got %v.", string(requiredResponse), w.Body.String())
	}

	testTime := time.Now()

	errCh := make(chan error, 1)

	dms.SubmitWriteRequest(storage.WriteRequest{
		Labels:         grouping1,
		Timestamp:      testTime,
		MetricFamilies: metricFamiliesMap(mf1, mf2),
		Done:           errCh,
	})

	for err := range errCh {
		t.Fatal("Unexpected error:", err)
	}

	w = httptest.NewRecorder()

	TestApi.metrics(w, req)

	requiredResponse, _ = json.Marshal(&response{
		Status: statusSuccess,
		Data:   templateMetrics(dms),
	})

	if expected, got := http.StatusOK, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}

	if !compareArray(requiredResponse, w.Body.Bytes()) {
		t.Errorf("Wanted response %v, got %v.", string(requiredResponse), w.Body.String())
	}
}

func metricFamiliesMap(mfs ...*dto.MetricFamily) map[string]*dto.MetricFamily {
	m := map[string]*dto.MetricFamily{}
	for _, mf := range mfs {
		buf, err := proto.Marshal(mf)
		if err != nil {
			panic(err)
		}
		mfCopy := &dto.MetricFamily{}
		if err := proto.Unmarshal(buf, mfCopy); err != nil {
			panic(err)
		}
		m[mf.GetName()] = mfCopy
	}
	return m
}
