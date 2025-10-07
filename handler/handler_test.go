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
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/route"
	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/pushgateway/storage"
)

var logger = promslog.NewNopLogger()

// MockMetricStore isn't doing any of the validation and sanitation a real
// metric store implementation has to do. Those are tested in the storage
// package. Here we only ensure that the right method calls are performed
// by the code in the handlers.
type MockMetricStore struct {
	lastWriteRequest storage.WriteRequest
	metricGroups     storage.GroupingKeyToMetricGroup
	writeRequests    []storage.WriteRequest
	err              error // If non-nil, will be sent to Done channel in request.
}

func (m *MockMetricStore) SubmitWriteRequest(req storage.WriteRequest) {
	m.writeRequests = append(m.writeRequests, req)
	m.lastWriteRequest = req
	if req.Done != nil {
		if m.err != nil {
			req.Done <- m.err
		}
		close(req.Done)
	}
}

func (m *MockMetricStore) GetMetricFamilies() []*dto.MetricFamily {
	panic("not implemented")
}

func (m *MockMetricStore) GetMetricFamiliesMap() storage.GroupingKeyToMetricGroup {
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

func ctxWithParams(params map[string]string, mainReq *http.Request) context.Context {
	ctx := mainReq.Context()

	for key, value := range params {
		ctx = route.WithParam(ctx, key, value)
	}

	return ctx
}

func TestHealthyReady(t *testing.T) {
	mms := MockMetricStore{}
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
	mms := MockMetricStore{}
	mmsWithErr := MockMetricStore{err: errors.New("testerror")}
	// false, true, false → no replace, check consistency, no base64 encoding.
	handler := Push(&mms, false, true, false, logger)
	handlerWithErr := Push(&mmsWithErr, false, true, false, logger)
	handlerBase64 := Push(&mms, false, true, true, logger)
	req, err := http.NewRequest("POST", "http://example.org/", &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}

	// No job name.
	w := httptest.NewRecorder()
	handler(w, req)
	if expected, got := http.StatusBadRequest, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
	if !mms.lastWriteRequest.Timestamp.IsZero() {
		t.Errorf("Write request timestamp unexpectedly set: %#v", mms.lastWriteRequest)
	}

	// With job name, but no instance name and no content.
	mms.lastWriteRequest = storage.WriteRequest{}
	w = httptest.NewRecorder()
	params := map[string]string{
		"job": "testjob",
	}

	handler(w, req.WithContext(ctxWithParams(params, req)))
	if expected, got := http.StatusOK, w.Code; expected != got {
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
	params = map[string]string{
		"job":      "testjob",
		"instance": "testinstance",
	}

	handler(w, req.WithContext(ctxWithParams(params, req)))
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
		bytes.NewBufferString("some_metric 3.14\nanother_metric{instance=\"testinstance\",job=\"testjob\"} 42\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()

	params = map[string]string{
		"job":    "testjob",
		"labels": "/instance/testinstance",
	}

	handler(w, req.WithContext(ctxWithParams(params, req)))
	if expected, got := http.StatusOK, w.Code; expected != got {
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
	// Note that sanitation hasn't happened yet, grouping labels not in request.
	verifyMetricFamily(t, `name:"some_metric" type:UNTYPED metric:{untyped:{value:3.14}}`, mms.lastWriteRequest.MetricFamilies["some_metric"])
	verifyMetricFamily(t, `name:"another_metric" type:UNTYPED metric:{label:{name:"instance" value:"testinstance"} label:{name:"job" value:"testjob"} untyped:{value:42}}`, mms.lastWriteRequest.MetricFamilies["another_metric"])

	// With job name and instance name and text content, storage returns error.
	req, err = http.NewRequest(
		"POST", "http://example.org/",
		bytes.NewBufferString("some_metric 3.14\nanother_metric{instance=\"testinstance\",job=\"testjob\"} 42\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	params = map[string]string{
		"job":    "testjob",
		"labels": "/instance/testinstance",
	}
	handlerWithErr(w, req.WithContext(ctxWithParams(params, req)))
	if expected, got := http.StatusBadRequest, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
	if mmsWithErr.lastWriteRequest.Timestamp.IsZero() {
		t.Errorf("Write request timestamp not set: %#v", mmsWithErr.lastWriteRequest)
	}
	if expected, got := "testjob", mmsWithErr.lastWriteRequest.Labels["job"]; expected != got {
		t.Errorf("Wanted job %v, got %v.", expected, got)
	}
	if expected, got := "testinstance", mmsWithErr.lastWriteRequest.Labels["instance"]; expected != got {
		t.Errorf("Wanted instance %v, got %v.", expected, got)
	}
	// Note that sanitation hasn't happened yet, grouping labels not in request.
	verifyMetricFamily(t, `name:"some_metric" type:UNTYPED metric:{untyped:{value:3.14}}`, mmsWithErr.lastWriteRequest.MetricFamilies["some_metric"])
	verifyMetricFamily(t, `name:"another_metric" type:UNTYPED metric:{label:{name:"instance" value:"testinstance"} label:{name:"job" value:"testjob"} untyped:{value:42}}`, mmsWithErr.lastWriteRequest.MetricFamilies["another_metric"])

	// With base64-encoded job name and instance name and text content.
	mms.lastWriteRequest = storage.WriteRequest{}
	req, err = http.NewRequest(
		"POST", "http://example.org/",
		bytes.NewBufferString("some_metric 3.14\nanother_metric{instance=\"testinstance\",job=\"testjob\"} 42\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	params = map[string]string{
		"job":    "dGVzdC9qb2I=",                      // job="test/job"
		"labels": "/instance@base64/dGVzdGluc3RhbmNl", // instance="testinstance"
	}
	handlerBase64(w, req.WithContext(ctxWithParams(params, req)))
	if expected, got := http.StatusOK, w.Code; expected != got {
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
	// Note that sanitation hasn't happened yet, grouping labels not in request.
	verifyMetricFamily(t, `name:"some_metric" type:UNTYPED metric:{untyped:{value:3.14}}`, mms.lastWriteRequest.MetricFamilies["some_metric"])
	// Note that sanitation hasn't happened yet, job label as still as in the push, not aligned to grouping labels.
	verifyMetricFamily(t, `name:"another_metric" type:UNTYPED metric:{label:{name:"instance" value:"testinstance"} label:{name:"job" value:"testjob"} untyped:{value:42}}`, mms.lastWriteRequest.MetricFamilies["another_metric"])

	// With job name and no instance name and text content.
	mms.lastWriteRequest = storage.WriteRequest{}
	req, err = http.NewRequest(
		"POST", "http://example.org/",
		bytes.NewBufferString("some_metric 3.14\nanother_metric{instance=\"testinstance\",job=\"testjob\"} 42\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	params = map[string]string{
		"job": "testjob",
	}
	handler(w, req.WithContext(ctxWithParams(params, req)))
	if expected, got := http.StatusOK, w.Code; expected != got {
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
	// Note that sanitation hasn't happened yet, grouping labels not in request.
	verifyMetricFamily(t, `name:"some_metric" type:UNTYPED metric:{untyped:{value:3.14}}`, mms.lastWriteRequest.MetricFamilies["some_metric"])
	verifyMetricFamily(t, `name:"another_metric" type:UNTYPED metric:{label:{name:"instance" value:"testinstance"} label:{name:"job" value:"testjob"} untyped:{value:42}}`, mms.lastWriteRequest.MetricFamilies["another_metric"])

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
	params = map[string]string{
		"job":    "testjob",
		"labels": "/instance/testinstance",
	}

	handler(w, req.WithContext(ctxWithParams(params, req)))
	// Note that a real storage shourd reject pushes with timestamps. Here
	// we only make sure it gets through. Rejection is tested in the storage
	// package.
	if expected, got := http.StatusOK, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
	// Make sure the timestamp from the push didn't make it to the WriteRequest.
	if time.Since(mms.lastWriteRequest.Timestamp) > time.Minute {
		t.Errorf("Write request timestamp set to a too low value: %#v", mms.lastWriteRequest)
	}
	if expected, got := int64(1000), mms.lastWriteRequest.MetricFamilies["b"].GetMetric()[0].GetTimestampMs(); expected != got {
		t.Errorf("Wanted protobuf timestamp %v, got %v.", expected, got)
	}

	// With job name and instance name and protobuf content.
	mms.lastWriteRequest = storage.WriteRequest{}
	buf := &bytes.Buffer{}
	_, err = protodelim.MarshalTo(buf, &dto.MetricFamily{
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

	_, err = protodelim.MarshalTo(buf, &dto.MetricFamily{
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

	_, err = protodelim.MarshalTo(buf, &dto.MetricFamily{
		Name: proto.String("histogram_metric"),
		Type: dto.MetricType_HISTOGRAM.Enum(),
		Metric: []*dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCountFloat: proto.Float64(20),
					SampleSum:        proto.Float64(99.23),
					Schema:           proto.Int32(1),
					NegativeCount:    []float64{2, 2, -2, 0},
					PositiveCount:    []float64{2, 2, -2, 0},
					PositiveSpan: []*dto.BucketSpan{
						{
							Offset: proto.Int32(0),
							Length: proto.Uint32(2),
						},
						{
							Offset: proto.Int32(0),
							Length: proto.Uint32(2),
						},
					},
					NegativeSpan: []*dto.BucketSpan{
						{
							Offset: proto.Int32(0),
							Length: proto.Uint32(2),
						},
						{
							Offset: proto.Int32(0),
							Length: proto.Uint32(2),
						},
					},
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
	params = map[string]string{
		"job":    "testjob",
		"labels": "/instance/testinstance",
	}
	handler(w, req.WithContext(ctxWithParams(params, req)))
	if expected, got := http.StatusOK, w.Code; expected != got {
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
	// Note that sanitation hasn't happened yet, grouping labels not in request.
	verifyMetricFamily(t, `name:"some_metric" type:UNTYPED metric:{untyped:{value:1.234}}`, mms.lastWriteRequest.MetricFamilies["some_metric"])
	// Note that sanitation hasn't happened yet, grouping labels not in request.
	verifyMetricFamily(t, `name:"another_metric" type:UNTYPED metric:{untyped:{value:3.14}}`, mms.lastWriteRequest.MetricFamilies["another_metric"])
	// Note that sanitation hasn't happened yet, grouping labels not in request.
	verifyMetricFamily(t, `name:"histogram_metric" type:HISTOGRAM metric:{histogram:{sample_count_float:20  sample_sum:99.23  schema:1  negative_span:{offset:0  length:2}  negative_span:{offset:0  length:2}  negative_count:2  negative_count:2  negative_count:-2  negative_count:0  positive_span:{offset:0  length:2}  positive_span:{offset:0  length:2}  positive_count:2  positive_count:2  positive_count:-2  positive_count:0}}`, mms.lastWriteRequest.MetricFamilies["histogram_metric"])
}

func TestPushUTF8(t *testing.T) {
	ValidationScheme = model.UTF8Validation
	EscapingScheme = model.ValueEncodingEscaping
	mms := MockMetricStore{}
	handler := Push(&mms, false, true, false, logger)
	handlerBase64 := Push(&mms, false, true, true, logger)

	// With job name, instance name, UTF-8 escaped label name in params, UTF-8 metric name and text content.
	mms.lastWriteRequest = storage.WriteRequest{}
	req, err := http.NewRequest(
		"POST", "http://example.org/",
		bytes.NewBufferString("some_metric 3.14\n{\"another.metric\",instance=\"testinstance\",job=\"testjob\",\"dotted.label.name\"=\"mylabelvalue\"} 42\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()

	params := map[string]string{
		"job":    "testjob",
		"labels": "/instance/testinstance/U__dotted_2e_label_2e_name/mylabelvalue",
	}

	handler(w, req.WithContext(ctxWithParams(params, req)))
	if expected, got := http.StatusOK, w.Code; expected != got {
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
	if expected, got := "mylabelvalue", mms.lastWriteRequest.Labels["dotted.label.name"]; expected != got {
		t.Errorf("Wanted dotted.label.name %v, got %v.", expected, got)
	}
	// Note that sanitation hasn't happened yet, grouping labels not in request.
	verifyMetricFamily(t, `name:"some_metric" type:UNTYPED metric:{untyped:{value:3.14}}`, mms.lastWriteRequest.MetricFamilies["some_metric"])
	verifyMetricFamily(t, `name:"another.metric" type:UNTYPED metric:{label:{name:"instance" value:"testinstance"} label:{name:"job" value:"testjob"} label:{name:"dotted.label.name" value:"mylabelvalue"} untyped:{value:42}}`, mms.lastWriteRequest.MetricFamilies["another.metric"])

	// With base64-encoded label values, UTF-8 escaped label name in params, UTF-8 metric name and text content.
	mms.lastWriteRequest = storage.WriteRequest{}
	req, err = http.NewRequest(
		"POST", "http://example.org/",
		bytes.NewBufferString("some_metric 3.14\n{\"another.metric\",instance=\"testinstance\",job=\"testjob\",\"dotted.label.name\"=\"mylabelvalue\"} 42\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	params = map[string]string{
		"job":    "dGVzdC9qb2I=",                                                                         // job="test/job"
		"labels": "/instance@base64/dGVzdGluc3RhbmNl/U__dotted_2e_label_2e_name@base64/bXlsYWJlbHZhbHVl", // instance="testinstance", dotted.label.name="mylabelvalue"
	}
	handlerBase64(w, req.WithContext(ctxWithParams(params, req)))
	if expected, got := http.StatusOK, w.Code; expected != got {
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
	if expected, got := "mylabelvalue", mms.lastWriteRequest.Labels["dotted.label.name"]; expected != got {
		t.Errorf("Wanted dotted.label.name %v, got %v.", expected, got)
	}
	// Note that sanitation hasn't happened yet, grouping labels not in request.
	verifyMetricFamily(t, `name:"some_metric" type:UNTYPED metric:{untyped:{value:3.14}}`, mms.lastWriteRequest.MetricFamilies["some_metric"])
	// Note that sanitation hasn't happened yet, job label as still as in the push, not aligned to grouping labels.
	verifyMetricFamily(t, `name:"another.metric" type:UNTYPED metric:{label:{name:"instance" value:"testinstance"} label:{name:"job" value:"testjob"} label:{name:"dotted.label.name" value:"mylabelvalue"} untyped:{value:42}}`, mms.lastWriteRequest.MetricFamilies["another.metric"])

	// With job name, instance name, UTF-8 escaped label name in params, UTF-8 metric names and protobuf content.
	mms.lastWriteRequest = storage.WriteRequest{}
	buf := &bytes.Buffer{}
	_, err = protodelim.MarshalTo(buf, &dto.MetricFamily{
		Name: proto.String("some.metric"),
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

	_, err = protodelim.MarshalTo(buf, &dto.MetricFamily{
		Name: proto.String("another.metric"),
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

	_, err = protodelim.MarshalTo(buf, &dto.MetricFamily{
		Name: proto.String("histogram.metric"),
		Type: dto.MetricType_HISTOGRAM.Enum(),
		Metric: []*dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCountFloat: proto.Float64(20),
					SampleSum:        proto.Float64(99.23),
					Schema:           proto.Int32(1),
					NegativeCount:    []float64{2, 2, -2, 0},
					PositiveCount:    []float64{2, 2, -2, 0},
					PositiveSpan: []*dto.BucketSpan{
						{
							Offset: proto.Int32(0),
							Length: proto.Uint32(2),
						},
						{
							Offset: proto.Int32(0),
							Length: proto.Uint32(2),
						},
					},
					NegativeSpan: []*dto.BucketSpan{
						{
							Offset: proto.Int32(0),
							Length: proto.Uint32(2),
						},
						{
							Offset: proto.Int32(0),
							Length: proto.Uint32(2),
						},
					},
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
	params = map[string]string{
		"job":    "testjob",
		"labels": "/instance/testinstance/U__dotted_2e_label_2e_name/mylabelvalue",
	}
	handler(w, req.WithContext(ctxWithParams(params, req)))
	if expected, got := http.StatusOK, w.Code; expected != got {
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
	if expected, got := "mylabelvalue", mms.lastWriteRequest.Labels["dotted.label.name"]; expected != got {
		t.Errorf("Wanted instance %v, got %v.", expected, got)
	}
	// Note that sanitation hasn't happened yet, grouping labels not in request.
	verifyMetricFamily(t, `name:"some.metric" type:UNTYPED metric:{untyped:{value:1.234}}`, mms.lastWriteRequest.MetricFamilies["some.metric"])
	// Note that sanitation hasn't happened yet, grouping labels not in request.
	verifyMetricFamily(t, `name:"another.metric" type:UNTYPED metric:{untyped:{value:3.14}}`, mms.lastWriteRequest.MetricFamilies["another.metric"])
	// Note that sanitation hasn't happened yet, grouping labels not in request.
	verifyMetricFamily(t, `name:"histogram.metric" type:HISTOGRAM metric:{histogram:{sample_count_float:20  sample_sum:99.23  schema:1  negative_span:{offset:0  length:2}  negative_span:{offset:0  length:2}  negative_count:2  negative_count:2  negative_count:-2  negative_count:0  positive_span:{offset:0  length:2}  positive_span:{offset:0  length:2}  positive_count:2  positive_count:2  positive_count:-2  positive_count:0}}`, mms.lastWriteRequest.MetricFamilies["histogram.metric"])

	ValidationScheme = model.LegacyValidation
	EscapingScheme = model.NoEscaping
}

func TestDelete(t *testing.T) {
	mms := MockMetricStore{}
	handler := Delete(&mms, false, logger)
	handlerBase64 := Delete(&mms, true, logger)
	req := &http.Request{}
	var params map[string]string

	// No job name.
	mms.lastWriteRequest = storage.WriteRequest{}
	w := httptest.NewRecorder()
	params = map[string]string{}
	handler(w, req.WithContext(ctxWithParams(params, req)))
	if expected, got := http.StatusBadRequest, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}
	if !mms.lastWriteRequest.Timestamp.IsZero() {
		t.Errorf("Write request timestamp unexpectedly set: %#v", mms.lastWriteRequest)
	}

	// With job name, but no instance name.
	mms.lastWriteRequest = storage.WriteRequest{}
	w = httptest.NewRecorder()

	params = map[string]string{
		"job": "testjob",
	}

	handler(w, req.WithContext(ctxWithParams(params, req)))
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

	params = map[string]string{
		"job":    "testjob",
		"labels": "/instance/testinstance",
	}

	handler(w, req.WithContext(ctxWithParams(params, req)))
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

	params = map[string]string{
		"job":    "dGVzdC9qb2I=",
		"labels": "/instance@base64/dGVzdGluc3RhbmNl",
	}

	handlerBase64(w, req.WithContext(ctxWithParams(params, req)))
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

func TestDeleteUTF8(t *testing.T) {
	ValidationScheme = model.UTF8Validation
	EscapingScheme = model.ValueEncodingEscaping
	mms := MockMetricStore{}
	handler := Delete(&mms, false, logger)
	handlerBase64 := Delete(&mms, true, logger)
	req := &http.Request{}
	var params map[string]string

	// With job name, instance name and UTF-8 escaped label name.
	mms.lastWriteRequest = storage.WriteRequest{}
	w := httptest.NewRecorder()

	params = map[string]string{
		"job":    "testjob",
		"labels": "/instance/testinstance/U__dotted_2e_label_2e_name/mylabelvalue",
	}

	handler(w, req.WithContext(ctxWithParams(params, req)))
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
	if expected, got := "mylabelvalue", mms.lastWriteRequest.Labels["dotted.label.name"]; expected != got {
		t.Errorf("Wanted instance %v, got %v.", expected, got)
	}

	// With base64-encoded label values and UTF-8 escaped label name.
	mms.lastWriteRequest = storage.WriteRequest{}
	w = httptest.NewRecorder()

	params = map[string]string{
		"job":    "dGVzdC9qb2I=",
		"labels": "/instance@base64/dGVzdGluc3RhbmNl/U__dotted_2e_label_2e_name@base64/bXlsYWJlbHZhbHVl",
	}

	handlerBase64(w, req.WithContext(ctxWithParams(params, req)))
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
	if expected, got := "mylabelvalue", mms.lastWriteRequest.Labels["dotted.label.name"]; expected != got {
		t.Errorf("Wanted instance %v, got %v.", expected, got)
	}

	ValidationScheme = model.LegacyValidation
	EscapingScheme = model.NoEscaping
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
		"regular label and UTF-8 escaped label name with legacy validation": {
			input: "/label_name1/label_value1/U__label_2e_name2/label_value2",
			expectedOutput: map[string]string{
				"label_name1":       "label_value1",
				"U__label_2e_name2": "label_value2",
			},
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

func TestSplitLabelsUTF8(t *testing.T) {
	scenarios := map[string]struct {
		input          string
		expectError    bool
		expectedOutput map[string]string
	}{
		"regular label and UTF-8 escaped label name": {
			input: "/label_name1/label_value1/U__label_2e_name2/label_value2",
			expectedOutput: map[string]string{
				"label_name1": "label_value1",
				"label.name2": "label_value2",
			},
		},
		"encoded slash in both label values and UTF-8 escaped label name": {
			input: "/label_name1@base64/bGFiZWwvdmFsdWUx/U__label_2e_name2@base64/bGFiZWwvdmFsdWUy",
			expectedOutput: map[string]string{
				"label_name1": "label/value1",
				"label.name2": "label/value2",
			},
		},
	}

	ValidationScheme = model.UTF8Validation
	EscapingScheme = model.ValueEncodingEscaping

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

	ValidationScheme = model.LegacyValidation
	EscapingScheme = model.NoEscaping
}

func TestWipeMetricStore(t *testing.T) {
	// Create MockMetricStore with a few GroupingKeyToMetricGroup metrics
	// so they can be returned by GetMetricFamiliesMap() to later send write
	// requests for each of them.
	metricCount := 5
	mgs := storage.GroupingKeyToMetricGroup{}
	for i := range metricCount {
		mgs[fmt.Sprint(i)] = storage.MetricGroup{}
	}
	mms := MockMetricStore{metricGroups: mgs}

	// Wipe handler should return 202 and delete all metrics.
	wipeHandler := WipeMetricStore(&mms, logger)
	w := httptest.NewRecorder()
	// Then handler is routed to the handler based on verb and path in main.go
	// therefore (and for now) we use the request to only record the returned status code.
	req, err := http.NewRequest("PUT", "http://example.org", &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	wipeHandler.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status code should be %d", http.StatusAccepted)
	}

	if len(mms.writeRequests) != metricCount {
		t.Errorf("there should be %d write requests, got %d instead", metricCount, len(mms.writeRequests))
	}

	// Were all the writeRequest deletes?.
	for i, wr := range mms.writeRequests {
		if wr.MetricFamilies != nil {
			t.Errorf("writeRequest at index %d was not a delete request", i)
		}
	}
}

// verifyMetricFamily jumps through a few hoops because the current protobuf
// implementation is deliberately creating an unstable formatting for the text
// representation. So this takes the text representation of the expected
// MetricFamily and unmarshals it into a proto message object first. Then it
// marshals both the expected and the got proto message into a binary protobuf,
// which it then compares.
func verifyMetricFamily(t *testing.T, expText string, got *dto.MetricFamily) {
	gotProto, err := proto.Marshal(got)
	if err != nil {
		t.Errorf("unexpected error marshaling MetricFamily %v", got)
	}

	exp := &dto.MetricFamily{}
	err = prototext.Unmarshal([]byte(expText), exp)
	if err != nil {
		t.Errorf("unexpected error unmarshaling MetricFamily text %v", expText)
	}
	expProto, err := proto.Marshal(exp)
	if err != nil {
		t.Errorf("unexpected error marshaling MetricFamily %v", exp)
	}

	if !bytes.Equal(expProto, gotProto) {
		t.Errorf("Wanted metric family %v, got %v.", exp, got)
	}
}
