// Copyright 2014 Prometheus Team
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
	"code.google.com/p/goprotobuf/proto"
	"github.com/julienschmidt/httprouter"
	"github.com/matttproud/golang_protobuf_extensions/ext"
	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/pushgateway/storage"
)

type MockMetricStore struct {
	lastWriteRequest storage.WriteRequest
}

func (m *MockMetricStore) SubmitWriteRequest(req storage.WriteRequest) {
	m.lastWriteRequest = req
}

func (m *MockMetricStore) GetMetricFamilies() []*dto.MetricFamily {
	panic("not implemented")
}

func (m *MockMetricStore) GetMetricFamiliesMap() storage.JobToInstanceMap {
	panic("not implemented")
}

func (m *MockMetricStore) Shutdown() error {
	return nil
}

func TestPush(t *testing.T) {
	mms := MockMetricStore{}
	handler := Push(&mms, false)
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
	if expected, got := "testjob", mms.lastWriteRequest.Job; expected != got {
		t.Errorf("Wanted job %v, got %v.", expected, got)
	}
	if expected, got := "localhost", mms.lastWriteRequest.Instance; expected != got {
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
	if expected, got := http.StatusInternalServerError, w.Code; expected != got {
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
			httprouter.Param{Key: "instance", Value: "testinstance"},
		},
	)
	if expected, got := http.StatusAccepted, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
	if mms.lastWriteRequest.Timestamp.IsZero() {
		t.Errorf("Write request timestamp not set: %#v", mms.lastWriteRequest)
	}
	if expected, got := "testjob", mms.lastWriteRequest.Job; expected != got {
		t.Errorf("Wanted job %v, got %v.", expected, got)
	}
	if expected, got := "testinstance", mms.lastWriteRequest.Instance; expected != got {
		t.Errorf("Wanted instance %v, got %v.", expected, got)
	}
	if expected, got := `name:"some_metric" type:UNTYPED metric:<label:<name:"job" value:"testjob" > label:<name:"instance" value:"testinstance" > untyped:<value:3.14 > > `, mms.lastWriteRequest.MetricFamilies["some_metric"].String(); expected != got {
		t.Errorf("Wanted metric family %v, got %v.", expected, got)
	}
	if expected, got := `name:"another_metric" type:UNTYPED metric:<label:<name:"job" value:"testjob" > label:<name:"instance" value:"testinstance" > untyped:<value:42 > > `, mms.lastWriteRequest.MetricFamilies["another_metric"].String(); expected != got {
		t.Errorf("Wanted metric family %v, got %v.", expected, got)
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
			httprouter.Param{Key: "instance", Value: "testinstance"},
		},
	)
	if expected, got := http.StatusAccepted, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
	if mms.lastWriteRequest.Timestamp.IsZero() {
		t.Errorf("Write request timestamp not set: %#v", mms.lastWriteRequest)
	}
	if expected, got := "testjob", mms.lastWriteRequest.Job; expected != got {
		t.Errorf("Wanted job %v, got %v.", expected, got)
	}
	if expected, got := "testinstance", mms.lastWriteRequest.Instance; expected != got {
		t.Errorf("Wanted instance %v, got %v.", expected, got)
	}
	if expected, got := `name:"some_metric" type:UNTYPED metric:<label:<name:"job" value:"testjob" > label:<name:"instance" value:"testinstance" > untyped:<value:3.14 > > `, mms.lastWriteRequest.MetricFamilies["some_metric"].String(); expected != got {
		t.Errorf("Wanted metric family %v, got %v.", expected, got)
	}
	if expected, got := `name:"another_metric" type:UNTYPED metric:<label:<name:"instance" value:"testinstance" > label:<name:"job" value:"testjob" > untyped:<value:42 > > `, mms.lastWriteRequest.MetricFamilies["another_metric"].String(); expected != got {
		t.Errorf("Wanted metric family %v, got %v.", expected, got)
	}

	// With job name and instance name and protobuf content.
	mms.lastWriteRequest = storage.WriteRequest{}
	buf := &bytes.Buffer{}
	_, err = ext.WriteDelimited(buf, &dto.MetricFamily{
		Name: proto.String("some_metric"),
		Type: dto.MetricType_UNTYPED.Enum(),
		Metric: []*dto.Metric{
			&dto.Metric{
				Untyped: &dto.Untyped{
					Value: proto.Float64(1.234),
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = ext.WriteDelimited(buf, &dto.MetricFamily{
		Name: proto.String("another_metric"),
		Type: dto.MetricType_UNTYPED.Enum(),
		Metric: []*dto.Metric{
			&dto.Metric{
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
			httprouter.Param{Key: "instance", Value: "testinstance"},
		},
	)
	if expected, got := http.StatusAccepted, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
	if mms.lastWriteRequest.Timestamp.IsZero() {
		t.Errorf("Write request timestamp not set: %#v", mms.lastWriteRequest)
	}
	if expected, got := "testjob", mms.lastWriteRequest.Job; expected != got {
		t.Errorf("Wanted job %v, got %v.", expected, got)
	}
	if expected, got := "testinstance", mms.lastWriteRequest.Instance; expected != got {
		t.Errorf("Wanted instance %v, got %v.", expected, got)
	}
	if expected, got := `name:"some_metric" type:UNTYPED metric:<label:<name:"job" value:"testjob" > label:<name:"instance" value:"testinstance" > untyped:<value:1.234 > > `, mms.lastWriteRequest.MetricFamilies["some_metric"].String(); expected != got {
		t.Errorf("Wanted metric family %v, got %v.", expected, got)
	}
	if expected, got := `name:"another_metric" type:UNTYPED metric:<label:<name:"job" value:"testjob" > label:<name:"instance" value:"testinstance" > untyped:<value:3.14 > > `, mms.lastWriteRequest.MetricFamilies["another_metric"].String(); expected != got {
		t.Errorf("Wanted metric family %v, got %v.", expected, got)
	}
}

func TestDelete(t *testing.T) {
	mms := MockMetricStore{}
	handler := Delete(&mms)

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
	if expected, got := "testjob", mms.lastWriteRequest.Job; expected != got {
		t.Errorf("Wanted job %v, got %v.", expected, got)
	}
	if expected, got := "", mms.lastWriteRequest.Instance; expected != got {
		t.Errorf("Wanted instance %v, got %v.", expected, got)
	}

	// With job name and instance name.
	mms.lastWriteRequest = storage.WriteRequest{}
	w = httptest.NewRecorder()
	handler(
		w, &http.Request{},
		httprouter.Params{
			httprouter.Param{Key: "job", Value: "testjob"},
			httprouter.Param{Key: "instance", Value: "testinstance"},
		},
	)
	if expected, got := http.StatusAccepted, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
	if mms.lastWriteRequest.Timestamp.IsZero() {
		t.Errorf("Write request timestamp not set: %#v", mms.lastWriteRequest)
	}
	if expected, got := "testjob", mms.lastWriteRequest.Job; expected != got {
		t.Errorf("Wanted job %v, got %v.", expected, got)
	}
	if expected, got := "testinstance", mms.lastWriteRequest.Instance; expected != got {
		t.Errorf("Wanted instance %v, got %v.", expected, got)
	}
}
