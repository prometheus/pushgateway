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
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-kit/kit/log"

	"github.com/golang/protobuf/proto"
	"github.com/julienschmidt/httprouter"
	"github.com/matttproud/golang_protobuf_extensions/pbutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"

	"github.com/prometheus/pushgateway/storage"
)

var logger = log.NewNopLogger()

type MockMetricStore struct {
	lastWriteRequest storage.WriteRequest
	metricGroups     storage.GroupingKeyToMetricGroup
}

func newMockMetricStore() MockMetricStore {
	return MockMetricStore{
		metricGroups: storage.GroupingKeyToMetricGroup{},
	}
}

// This mock method just set lastWriteRequest and write metrics, but some fields
// are set to its zero value
func (m *MockMetricStore) SubmitWriteRequest(req storage.WriteRequest) {
	m.lastWriteRequest = req

	key := model.LabelsToSignature(req.Labels)
	// Delete when MetricFamilies == nil
	if req.MetricFamilies == nil {
		delete(m.metricGroups, key)
		return
	}

	// Update metric contains similar logic as you find
	// within diskmetricstore.processWriteRequest()
	for name, _ := range req.MetricFamilies {
		group, ok := m.metricGroups[key]
		if !ok {
			group = storage.MetricGroup{
				Labels:  req.Labels,
				Metrics: storage.NameToTimestampedMetricFamilyMap{},
			}
			m.metricGroups[key] = group
		}
		group.Metrics[name] = storage.TimestampedMetricFamily{
			Timestamp: req.Timestamp,
		}
	}

}

func (m *MockMetricStore) GetMetricFamilies() []*dto.MetricFamily {
	panic("not implemented")
}

func (m *MockMetricStore) GetMetricFamiliesMap() storage.GroupingKeyToMetricGroup {
	// Simply return them without performing a copy
	return m.metricGroups
}

func (m *MockMetricStore) Shutdown() error {
	return nil
}

func (m *MockMetricStore) Healthy() error {
	return nil
}

func (m *MockMetricStore) Ready() error {
	return nil
}

func TestWipeMetricStore(t *testing.T) {
	mms := newMockMetricStore()
	pushHandler := Push(&mms, false, false, logger)

	req, err := http.NewRequest(
		"POST", "http://example.org/",
		bytes.NewBufferString("some_metric 3.14\nanother_metric 42\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()

	// Push a few metrics to the MetricStore
	count := 10
	for i := 0; i < count; i++ {
		pushHandler(
			w, req,
			httprouter.Params{
				httprouter.Param{Key: "job", Value: "testjob" + string(i)},
				httprouter.Param{Key: "labels", Value: "/instance/testinstance"},
			},
		)
	}

	// Just a basic checking to ensure MockMetricStore was filled up correctly
	if len(mms.GetMetricFamiliesMap()) != count {
		t.Errorf("Length should be %d, got %d instead", count, len(mms.GetMetricFamiliesMap()))
	}

	// Wipe handler should return 202 and delete all metrics
	wipeHandler := WipeMetricStore(&mms, logger)
	w = httptest.NewRecorder()
	wipeHandler.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status code should be %d", http.StatusAccepted)
	}

	if len(mms.GetMetricFamiliesMap()) != 0 {
		t.Errorf("Length should be %d, got %d instead", 0, len(mms.GetMetricFamiliesMap()))
	}
}

func TestHealthyReady(t *testing.T) {
	mms := newMockMetricStore()
	req, err := http.NewRequest("GET", "http://example.org/", &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}

	healthyHandler := Healthy(&mms)
	readyHandler := Ready(&mms)

	w := httptest.NewRecorder()
	healthyHandler.ServeHTTP(w, req)
	if expected, got := http.StatusOK, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}

	readyHandler.ServeHTTP(w, req)
	if expected, got := http.StatusOK, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
}

func TestPush(t *testing.T) {
	mms := newMockMetricStore()
	handler := Push(&mms, false, false, logger)
	handlerBase64 := Push(&mms, false, true, logger)
	req, err := http.NewRequest("POST", "http://example.org/", &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}

	// No job name.
	w := httptest.NewRecorder()
	handler(w, req, httprouter.Params{})
	if expected, got := http.StatusBadRequest, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
	if !mms.lastWriteRequest.Timestamp.IsZero() {
		t.Errorf("Write request timestamp unexpectedly set: %#v", mms.lastWriteRequest)
	}

	// With job name, but no instance name and no content.
	mms.lastWriteRequest = storage.WriteRequest{}
	w = httptest.NewRecorder()
	handler(w, req, httprouter.Params{httprouter.Param{Key: "job", Value: "testjob"}})
	if expected, got := http.StatusAccepted, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
	if mms.lastWriteRequest.Timestamp.IsZero() {
		t.Errorf("Write request timestamp not set: %#v", mms.lastWriteRequest)
	}
	if expected, got := "testjob", mms.lastWriteRequest.Labels["job"]; expected != got {
		t.Errorf("Wanted job %v, got %v.", expected, got)
	}
	if expected, got := "", mms.lastWriteRequest.Labels["instance"]; expected != got {
		t.Errorf("Wanted instance %v, got %v.", expected, got)
	}

	// With job name and instance name and invalid text content.
	mms.lastWriteRequest = storage.WriteRequest{}
	req, err = http.NewRequest(
		"POST", "http://example.org/",
		bytes.NewBufferString("blablabla\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	handler(
		w, req,
		httprouter.Params{
			httprouter.Param{Key: "job", Value: "testjob"},
			httprouter.Param{Key: "instance", Value: "testinstance"},
		},
	)
	if expected, got := http.StatusBadRequest, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
	if !mms.lastWriteRequest.Timestamp.IsZero() {
		t.Errorf("Write request timestamp unexpectedly set: %#v", mms.lastWriteRequest)
	}

	// With job name and instance name and text content.
	mms.lastWriteRequest = storage.WriteRequest{}
	req, err = http.NewRequest(
		"POST", "http://example.org/",
		bytes.NewBufferString("some_metric 3.14\nanother_metric 42\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	handler(
		w, req,
		httprouter.Params{
			httprouter.Param{Key: "job", Value: "testjob"},
			httprouter.Param{Key: "labels", Value: "/instance/testinstance"},
		},
	)
	if expected, got := http.StatusAccepted, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
	if mms.lastWriteRequest.Timestamp.IsZero() {
		t.Errorf("Write request timestamp not set: %#v", mms.lastWriteRequest)
	}
	if expected, got := "testjob", mms.lastWriteRequest.Labels["job"]; expected != got {
		t.Errorf("Wanted job %v, got %v.", expected, got)
	}
	if expected, got := "testinstance", mms.lastWriteRequest.Labels["instance"]; expected != got {
		t.Errorf("Wanted instance %v, got %v.", expected, got)
	}
	if expected, got := `name:"some_metric" type:UNTYPED metric:<label:<name:"instance" value:"testinstance" > label:<name:"job" value:"testjob" > untyped:<value:3.14 > > `, mms.lastWriteRequest.MetricFamilies["some_metric"].String(); expected != got {
		t.Errorf("Wanted metric family %v, got %v.", expected, got)
	}
	if expected, got := `name:"another_metric" type:UNTYPED metric:<label:<name:"instance" value:"testinstance" > label:<name:"job" value:"testjob" > untyped:<value:42 > > `, mms.lastWriteRequest.MetricFamilies["another_metric"].String(); expected != got {
		t.Errorf("Wanted metric family %v, got %v.", expected, got)
	}
	if _, ok := mms.lastWriteRequest.MetricFamilies["push_time_seconds"]; !ok {
		t.Errorf("Wanted metric family push_time_seconds missing.")
	}

	// With base64-encoded job name and instance name and text content.
	mms.lastWriteRequest = storage.WriteRequest{}
	req, err = http.NewRequest(
		"POST", "http://example.org/",
		bytes.NewBufferString("some_metric 3.14\nanother_metric 42\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	handlerBase64(
		w, req,
		httprouter.Params{
			httprouter.Param{Key: "job", Value: "dGVzdC9qb2I="},                         // job="test/job"
			httprouter.Param{Key: "labels", Value: "/instance@base64/dGVzdGluc3RhbmNl"}, // instance="testinstance"
		},
	)
	if expected, got := http.StatusAccepted, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
	if mms.lastWriteRequest.Timestamp.IsZero() {
		t.Errorf("Write request timestamp not set: %#v", mms.lastWriteRequest)
	}
	if expected, got := "test/job", mms.lastWriteRequest.Labels["job"]; expected != got {
		t.Errorf("Wanted job %v, got %v.", expected, got)
	}
	if expected, got := "testinstance", mms.lastWriteRequest.Labels["instance"]; expected != got {
		t.Errorf("Wanted instance %v, got %v.", expected, got)
	}
	if expected, got := `name:"some_metric" type:UNTYPED metric:<label:<name:"instance" value:"testinstance" > label:<name:"job" value:"test/job" > untyped:<value:3.14 > > `, mms.lastWriteRequest.MetricFamilies["some_metric"].String(); expected != got {
		t.Errorf("Wanted metric family %v, got %v.", expected, got)
	}
	if expected, got := `name:"another_metric" type:UNTYPED metric:<label:<name:"instance" value:"testinstance" > label:<name:"job" value:"test/job" > untyped:<value:42 > > `, mms.lastWriteRequest.MetricFamilies["another_metric"].String(); expected != got {
		t.Errorf("Wanted metric family %v, got %v.", expected, got)
	}
	if _, ok := mms.lastWriteRequest.MetricFamilies["push_time_seconds"]; !ok {
		t.Errorf("Wanted metric family push_time_seconds missing.")
	}

	// With job name and no instance name and text content.
	mms.lastWriteRequest = storage.WriteRequest{}
	req, err = http.NewRequest(
		"POST", "http://example.org/",
		bytes.NewBufferString("some_metric 3.14\nanother_metric 42\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	handler(
		w, req,
		httprouter.Params{
			httprouter.Param{Key: "job", Value: "testjob"},
		},
	)
	if expected, got := http.StatusAccepted, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
	if mms.lastWriteRequest.Timestamp.IsZero() {
		t.Errorf("Write request timestamp not set: %#v", mms.lastWriteRequest)
	}
	if expected, got := "testjob", mms.lastWriteRequest.Labels["job"]; expected != got {
		t.Errorf("Wanted job %v, got %v.", expected, got)
	}
	if expected, got := "", mms.lastWriteRequest.Labels["instance"]; expected != got {
		t.Errorf("Wanted instance %v, got %v.", expected, got)
	}
	if expected, got := `name:"some_metric" type:UNTYPED metric:<label:<name:"instance" value:"" > label:<name:"job" value:"testjob" > untyped:<value:3.14 > > `, mms.lastWriteRequest.MetricFamilies["some_metric"].String(); expected != got {
		t.Errorf("Wanted metric family %v, got %v.", expected, got)
	}
	if expected, got := `name:"another_metric" type:UNTYPED metric:<label:<name:"instance" value:"" > label:<name:"job" value:"testjob" > untyped:<value:42 > > `, mms.lastWriteRequest.MetricFamilies["another_metric"].String(); expected != got {
		t.Errorf("Wanted metric family %v, got %v.", expected, got)
	}

	// With job name and instance name and timestamp specified.
	mms.lastWriteRequest = storage.WriteRequest{}
	req, err = http.NewRequest(
		"POST", "http://example.org/",
		bytes.NewBufferString("a 1\nb 1 1000\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	handler(
		w, req,
		httprouter.Params{
			httprouter.Param{Key: "job", Value: "testjob"},
			httprouter.Param{Key: "labels", Value: "/instance/testinstance"},
		},
	)
	if expected, got := http.StatusBadRequest, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
	if !mms.lastWriteRequest.Timestamp.IsZero() {
		t.Errorf("Write request timestamp unexpectedly set: %#v", mms.lastWriteRequest)
	}

	// With job name and instance name and text content and job and instance labels.
	mms.lastWriteRequest = storage.WriteRequest{}
	req, err = http.NewRequest(
		"POST", "http://example.org",
		bytes.NewBufferString(`
some_metric{job="foo",instance="bar"} 3.14
another_metric{instance="baz"} 42
`),
	)
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	handler(
		w, req,
		httprouter.Params{
			httprouter.Param{Key: "job", Value: "testjob"},
			httprouter.Param{Key: "labels", Value: "/instance/testinstance"},
		},
	)
	if expected, got := http.StatusAccepted, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
	if mms.lastWriteRequest.Timestamp.IsZero() {
		t.Errorf("Write request timestamp not set: %#v", mms.lastWriteRequest)
	}
	if expected, got := "testjob", mms.lastWriteRequest.Labels["job"]; expected != got {
		t.Errorf("Wanted job %v, got %v.", expected, got)
	}
	if expected, got := "testinstance", mms.lastWriteRequest.Labels["instance"]; expected != got {
		t.Errorf("Wanted instance %v, got %v.", expected, got)
	}
	if expected, got := `name:"some_metric" type:UNTYPED metric:<label:<name:"instance" value:"testinstance" > label:<name:"job" value:"testjob" > untyped:<value:3.14 > > `, mms.lastWriteRequest.MetricFamilies["some_metric"].String(); expected != got {
		t.Errorf("Wanted metric family %v, got %v.", expected, got)
	}
	if expected, got := `name:"another_metric" type:UNTYPED metric:<label:<name:"instance" value:"testinstance" > label:<name:"job" value:"testjob" > untyped:<value:42 > > `, mms.lastWriteRequest.MetricFamilies["another_metric"].String(); expected != got {
		t.Errorf("Wanted metric family %v, got %v.", expected, got)
	}

	// With job name and instance name and protobuf content.
	mms.lastWriteRequest = storage.WriteRequest{}
	buf := &bytes.Buffer{}
	_, err = pbutil.WriteDelimited(buf, &dto.MetricFamily{
		Name: proto.String("some_metric"),
		Type: dto.MetricType_UNTYPED.Enum(),
		Metric: []*dto.Metric{
			{
				Untyped: &dto.Untyped{
					Value: proto.Float64(1.234),
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = pbutil.WriteDelimited(buf, &dto.MetricFamily{
		Name: proto.String("another_metric"),
		Type: dto.MetricType_UNTYPED.Enum(),
		Metric: []*dto.Metric{
			{
				Untyped: &dto.Untyped{
					Value: proto.Float64(3.14),
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	req, err = http.NewRequest(
		"POST", "http://example.org/", buf,
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/vnd.google.protobuf; encoding=delimited; proto=io.prometheus.client.MetricFamily")
	w = httptest.NewRecorder()
	handler(
		w, req,
		httprouter.Params{
			httprouter.Param{Key: "job", Value: "testjob"},
			httprouter.Param{Key: "labels", Value: "/instance/testinstance"},
		},
	)
	if expected, got := http.StatusAccepted, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
	if mms.lastWriteRequest.Timestamp.IsZero() {
		t.Errorf("Write request timestamp not set: %#v", mms.lastWriteRequest)
	}
	if expected, got := "testjob", mms.lastWriteRequest.Labels["job"]; expected != got {
		t.Errorf("Wanted job %v, got %v.", expected, got)
	}
	if expected, got := "testinstance", mms.lastWriteRequest.Labels["instance"]; expected != got {
		t.Errorf("Wanted instance %v, got %v.", expected, got)
	}
	if expected, got := `name:"some_metric" type:UNTYPED metric:<label:<name:"instance" value:"testinstance" > label:<name:"job" value:"testjob" > untyped:<value:1.234 > > `, mms.lastWriteRequest.MetricFamilies["some_metric"].String(); expected != got {
		t.Errorf("Wanted metric family %v, got %v.", expected, got)
	}
	if expected, got := `name:"another_metric" type:UNTYPED metric:<label:<name:"instance" value:"testinstance" > label:<name:"job" value:"testjob" > untyped:<value:3.14 > > `, mms.lastWriteRequest.MetricFamilies["another_metric"].String(); expected != got {
		t.Errorf("Wanted metric family %v, got %v.", expected, got)
	}
}

func TestDelete(t *testing.T) {
	mms := newMockMetricStore()
	handler := Delete(&mms, false, logger)
	handlerBase64 := Delete(&mms, true, logger)

	// No job name.
	mms.lastWriteRequest = storage.WriteRequest{}
	w := httptest.NewRecorder()
	handler(
		w, &http.Request{},
		httprouter.Params{},
	)
	if expected, got := http.StatusBadRequest, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
	if !mms.lastWriteRequest.Timestamp.IsZero() {
		t.Errorf("Write request timestamp unexpectedly set: %#v", mms.lastWriteRequest)
	}

	// With job name, but no instance name.
	mms.lastWriteRequest = storage.WriteRequest{}
	w = httptest.NewRecorder()
	handler(
		w, &http.Request{},
		httprouter.Params{
			httprouter.Param{Key: "job", Value: "testjob"},
		},
	)
	if expected, got := http.StatusAccepted, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
	if mms.lastWriteRequest.Timestamp.IsZero() {
		t.Errorf("Write request timestamp not set: %#v", mms.lastWriteRequest)
	}
	if expected, got := "testjob", mms.lastWriteRequest.Labels["job"]; expected != got {
		t.Errorf("Wanted job %v, got %v.", expected, got)
	}
	if expected, got := "", mms.lastWriteRequest.Labels["instance"]; expected != got {
		t.Errorf("Wanted instance %v, got %v.", expected, got)
	}

	// With job name and instance name.
	mms.lastWriteRequest = storage.WriteRequest{}
	w = httptest.NewRecorder()
	handler(
		w, &http.Request{},
		httprouter.Params{
			httprouter.Param{Key: "job", Value: "testjob"},
			httprouter.Param{Key: "labels", Value: "/instance/testinstance"},
		},
	)
	if expected, got := http.StatusAccepted, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
	if mms.lastWriteRequest.Timestamp.IsZero() {
		t.Errorf("Write request timestamp not set: %#v", mms.lastWriteRequest)
	}
	if expected, got := "testjob", mms.lastWriteRequest.Labels["job"]; expected != got {
		t.Errorf("Wanted job %v, got %v.", expected, got)
	}
	if expected, got := "testinstance", mms.lastWriteRequest.Labels["instance"]; expected != got {
		t.Errorf("Wanted instance %v, got %v.", expected, got)
	}

	// With base64-encoded job name and instance name.
	mms.lastWriteRequest = storage.WriteRequest{}
	w = httptest.NewRecorder()
	handlerBase64(
		w, &http.Request{},
		httprouter.Params{
			httprouter.Param{Key: "job", Value: "dGVzdC9qb2I="},                         // job="test/job"
			httprouter.Param{Key: "labels", Value: "/instance@base64/dGVzdGluc3RhbmNl"}, // instance="testinstance"
		},
	)
	if expected, got := http.StatusAccepted, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
	if mms.lastWriteRequest.Timestamp.IsZero() {
		t.Errorf("Write request timestamp not set: %#v", mms.lastWriteRequest)
	}
	if expected, got := "test/job", mms.lastWriteRequest.Labels["job"]; expected != got {
		t.Errorf("Wanted job %v, got %v.", expected, got)
	}
	if expected, got := "testinstance", mms.lastWriteRequest.Labels["instance"]; expected != got {
		t.Errorf("Wanted instance %v, got %v.", expected, got)
	}

}

func TestSplitLabels(t *testing.T) {
	scenarios := map[string]struct {
		input          string
		expectError    bool
		expectedOutput map[string]string
	}{
		"regular labels": {
			input: "/label_name1/label_value1/label_name2/label_value2",
			expectedOutput: map[string]string{
				"label_name1": "label_value1",
				"label_name2": "label_value2",
			},
		},
		"invalid label name": {
			input:       "/label_name1/label_value1/a=b/label_value2",
			expectError: true,
		},
		"reserved label name": {
			input:       "/label_name1/label_value1/__label_name2/label_value2",
			expectError: true,
		},
		"unencoded slash in label value": {
			input:       "/label_name1/label_value1/label_name2/label/value2",
			expectError: true,
		},
		"encoded slash in first label value ": {
			input: "/label_name1@base64/bGFiZWwvdmFsdWUx/label_name2/label_value2",
			expectedOutput: map[string]string{
				"label_name1": "label/value1",
				"label_name2": "label_value2",
			},
		},
		"encoded slash in last label value": {
			input: "/label_name1/label_value1/label_name2@base64/bGFiZWwvdmFsdWUy",
			expectedOutput: map[string]string{
				"label_name1": "label_value1",
				"label_name2": "label/value2",
			},
		},
		"encoded slash in last label value with padding": {
			input: "/label_name1/label_value1/label_name2@base64/bGFiZWwvdmFsdWUy==",
			expectedOutput: map[string]string{
				"label_name1": "label_value1",
				"label_name2": "label/value2",
			},
		},
		"invalid base64 encoding": {
			input:       "/label_name1@base64/foo.bar/label_name2/label_value2",
			expectError: true,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			parsed, err := splitLabels(scenario.input)
			if err != nil {
				if scenario.expectError {
					return // All good.
				}
				t.Fatalf("Got unexpected error: %s.", err)
			}
			for k, v := range scenario.expectedOutput {
				got, ok := parsed[k]
				if !ok {
					t.Errorf("Expected to find %s=%q.", k, v)
				}
				if got != v {
					t.Errorf("Expected %s=%q but got %s=%q.", k, v, k, got)
				}
				delete(parsed, k)
			}
			for k, v := range parsed {
				t.Errorf("Found unexpected label %s=%q.", k, v)
			}
		})
	}
}
