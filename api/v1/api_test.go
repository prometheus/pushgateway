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
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	//nolint:staticcheck // Ignore SA1019. Dependencies use the deprecated package, so we have to, too.
	"github.com/golang/protobuf/proto"
	"github.com/prometheus/common/promslog"

	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/pushgateway/storage"
	"github.com/prometheus/pushgateway/testutil"
)

var (
	logger    = promslog.NewNopLogger()
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
		Type: dto.MetricType_SUMMARY.Enum(),
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("instance"),
						Value: proto.String(`inst'a"n\ce1`),
					},
					{
						Name:  proto.String("job"),
						Value: proto.String("Björn"),
					},
				},
				Summary: &dto.Summary{
					SampleCount: proto.Uint64(0),
					SampleSum:   proto.Float64(0),
				},
			},
		},
	}
	mfh = &dto.MetricFamily{
		Name: proto.String("mfh"),
		Type: dto.MetricType_HISTOGRAM.Enum(),
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("testing"),
						Value: proto.String("int classic histogram"),
					},
				},
				Histogram: &dto.Histogram{
					SampleCount: proto.Uint64(20),
					SampleSum:   proto.Float64(99.23),
					Bucket: []*dto.Bucket{
						{
							UpperBound:      proto.Float64(250000),
							CumulativeCount: proto.Uint64(3),
						},
						{
							UpperBound:      proto.Float64(500000),
							CumulativeCount: proto.Uint64(17),
						},
					},
				},
			},
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("testing"),
						Value: proto.String("float classic histogram"),
					},
				},
				Histogram: &dto.Histogram{
					SampleCountFloat: proto.Float64(20),
					SampleSum:        proto.Float64(99.23),
					Bucket: []*dto.Bucket{
						{
							UpperBound:           proto.Float64(250000),
							CumulativeCountFloat: proto.Float64(3),
						},
						{
							UpperBound:           proto.Float64(500000),
							CumulativeCountFloat: proto.Float64(17),
						},
					},
				},
			},
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("testing"),
						Value: proto.String("int native histogram"),
					},
				},
				Histogram: &dto.Histogram{
					SampleCount:   proto.Uint64(20),
					SampleSum:     proto.Float64(99.23),
					Schema:        proto.Int32(1),
					NegativeDelta: []int64{0, 2, -2, 0},
					PositiveDelta: []int64{0, 2, -2, 0},
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
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("testing"),
						Value: proto.String("float native histogram"),
					},
				},
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
	}

	grouping1 = map[string]string{
		"job":      "Björn",
		"instance": `inst'a"n\ce1`,
	}
)

func convertMap(m map[string]string) map[string]interface{} {
	result := map[string]interface{}{}
	for k, v := range m {
		result[k] = v
	}
	return result
}

func TestStatusAPI(t *testing.T) {
	dms := storage.NewDiskMetricStore("", 100*time.Millisecond, nil, logger)
	testAPI := New(logger, dms, testFlags, testBuildInfo)

	req, err := http.NewRequest("GET", "http://example.org/", &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()

	testResponse := response{}
	testAPI.status(w, req)
	json.Unmarshal(w.Body.Bytes(), &testResponse)
	jsonData := testResponse.Data.(map[string]interface{})
	responseFlagData := jsonData["flags"].(map[string]interface{})
	responseBuildInfo := jsonData["build_information"].(map[string]interface{})

	if expected, got := http.StatusOK, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}

	if !reflect.DeepEqual(responseFlagData, convertMap(testFlags)) {
		t.Errorf("Wanted following flags %q, got %q.", testFlags, responseFlagData)
	}

	if !reflect.DeepEqual(responseBuildInfo, convertMap(testBuildInfo)) {
		t.Errorf("Wanted following build info %q, got %q.", testBuildInfo, responseBuildInfo)
	}
}

func TestMetricsAPI(t *testing.T) {
	dms := storage.NewDiskMetricStore("", 100*time.Millisecond, nil, logger)
	testAPI := New(logger, dms, testFlags, testBuildInfo)

	req, err := http.NewRequest("GET", "http://example.org/", &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()

	testAPI.metrics(w, req)

	if expected, got := http.StatusOK, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}

	requiredResponse := `{"status":"success","data":[]}`

	if expected, got := requiredResponse, w.Body.String(); expected != got {
		t.Errorf("Wanted response %q, got %q.", requiredResponse, w.Body.String())
	}

	testTime, _ := time.Parse(time.RFC3339Nano, "2020-03-10T00:54:08.025744841+05:30")

	errCh := make(chan error, 1)

	dms.SubmitWriteRequest(storage.WriteRequest{
		Labels:         grouping1,
		Timestamp:      testTime,
		MetricFamilies: testutil.MetricFamiliesMap(mf1, mfh),
		Done:           errCh,
	})

	for err := range errCh {
		t.Fatal("Unexpected error:", err)
	}

	w = httptest.NewRecorder()

	testAPI.metrics(w, req)

	var prettyJSON bytes.Buffer
	json.Indent(&prettyJSON, w.Body.Bytes(), "", "\t")

	requiredResponse = `{
	"status": "success",
	"data": [
		{
			"labels": {
				"instance": "inst'a\"n\\ce1",
				"job": "Björn"
			},
			"last_push_successful": true,
			"mf1": {
				"time_stamp": "2020-03-10T00:54:08.025744841+05:30",
				"type": "SUMMARY",
				"metrics": [
					{
						"count": "0",
						"labels": {
							"instance": "inst'a\"n\\ce1",
							"job": "Björn"
						},
						"quantiles": {},
						"sum": "0"
					}
				]
			},
			"mfh": {
				"time_stamp": "2020-03-10T00:54:08.025744841+05:30",
				"type": "HISTOGRAM",
				"metrics": [
					{
						"buckets": {
							"250000": "3",
							"500000": "17"
						},
						"count": "20",
						"labels": {
							"instance": "inst'a\"n\\ce1",
							"job": "Björn",
							"testing": "int classic histogram"
						},
						"sum": "99.23"
					},
					{
						"buckets": {
							"250000": "3",
							"500000": "17"
						},
						"count": "20",
						"labels": {
							"instance": "inst'a\"n\\ce1",
							"job": "Björn",
							"testing": "float classic histogram"
						},
						"sum": "99.23"
					},
					{
						"buckets": [
							[
								1,
								"-1.414213562373095",
								"-1",
								"2"
							],
							[
								0,
								"1",
								"1.414213562373095",
								"2"
							]
						],
						"count": "20",
						"labels": {
							"instance": "inst'a\"n\\ce1",
							"job": "Björn",
							"testing": "int native histogram"
						},
						"sum": "99.23"
					},
					{
						"buckets": [
							[
								1,
								"-2",
								"-1.414213562373095",
								"-2"
							],
							[
								1,
								"-1.414213562373095",
								"-1",
								"2"
							],
							[
								1,
								"-1",
								"-0.7071067811865475",
								"2"
							],
							[
								0,
								"0.7071067811865475",
								"1",
								"2"
							],
							[
								0,
								"1",
								"1.414213562373095",
								"2"
							],
							[
								0,
								"1.414213562373095",
								"2",
								"-2"
							]
						],
						"count": "20",
						"labels": {
							"instance": "inst'a\"n\\ce1",
							"job": "Björn",
							"testing": "float native histogram"
						},
						"sum": "99.23"
					}
				]
			},
			"push_failure_time_seconds": {
				"time_stamp": "2020-03-10T00:54:08.025744841+05:30",
				"type": "GAUGE",
				"help": "Last Unix time when changing this group in the Pushgateway failed.",
				"metrics": [
					{
						"labels": {
							"instance": "inst'a\"n\\ce1",
							"job": "Björn"
						},
						"value": "0"
					}
				]
			},
			"push_time_seconds": {
				"time_stamp": "2020-03-10T00:54:08.025744841+05:30",
				"type": "GAUGE",
				"help": "Last Unix time when changing this group in the Pushgateway succeeded.",
				"metrics": [
					{
						"labels": {
							"instance": "inst'a\"n\\ce1",
							"job": "Björn"
						},
						"value": "1.583781848025745e+09"
					}
				]
			}
		}
	]
}`

	if expected, got := http.StatusOK, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}

	if expected, got := requiredResponse, prettyJSON.String(); expected != got {
		t.Errorf("Wanted response %q, got %q.", expected, got)
	}
}
