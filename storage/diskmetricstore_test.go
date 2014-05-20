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

package storage

import (
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path"
	"sort"
	"testing"
	"time"

	"code.google.com/p/goprotobuf/proto"

	dto "github.com/prometheus/client_model/go"
)

var (
	// Example metric families.
	mf1a = &dto.MetricFamily{
		Name: proto.String("mf1"),
		Type: dto.MetricType_UNTYPED.Enum(),
		Metric: []*dto.Metric{
			&dto.Metric{
				Label: []*dto.LabelPair{
					&dto.LabelPair{
						Name:  proto.String("job"),
						Value: proto.String("job1"),
					},
					&dto.LabelPair{
						Name:  proto.String("instance"),
						Value: proto.String("instance2"),
					},
				},
				Untyped: &dto.Untyped{
					Value: proto.Float64(-3e3),
				},
				TimestampMs: proto.Int64(103948),
			},
		},
	}
	mf1b = &dto.MetricFamily{
		Name: proto.String("mf1"),
		Type: dto.MetricType_UNTYPED.Enum(),
		Metric: []*dto.Metric{
			&dto.Metric{
				Label: []*dto.LabelPair{
					&dto.LabelPair{
						Name:  proto.String("job"),
						Value: proto.String("job1"),
					},
					&dto.LabelPair{
						Name:  proto.String("instance"),
						Value: proto.String("instance2"),
					},
				},
				Untyped: &dto.Untyped{
					Value: proto.Float64(42),
				},
			},
		},
	}
	mf2 = &dto.MetricFamily{
		Name: proto.String("mf2"),
		Help: proto.String("doc string 2"),
		Type: dto.MetricType_GAUGE.Enum(),
		Metric: []*dto.Metric{
			&dto.Metric{
				Label: []*dto.LabelPair{
					&dto.LabelPair{
						Name:  proto.String("job"),
						Value: proto.String("job1"),
					},
					&dto.LabelPair{
						Name:  proto.String("instance"),
						Value: proto.String("instance2"),
					},
					&dto.LabelPair{
						Name:  proto.String("labelname"),
						Value: proto.String("val2"),
					},
					&dto.LabelPair{
						Name:  proto.String("basename"),
						Value: proto.String("basevalue2"),
					},
				},
				Gauge: &dto.Gauge{
					Value: proto.Float64(math.Inf(+1)),
				},
				TimestampMs: proto.Int64(54321),
			},
			&dto.Metric{
				Label: []*dto.LabelPair{
					&dto.LabelPair{
						Name:  proto.String("job"),
						Value: proto.String("job1"),
					},
					&dto.LabelPair{
						Name:  proto.String("instance"),
						Value: proto.String("instance2"),
					},
					&dto.LabelPair{
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
	mf3 = &dto.MetricFamily{
		Name: proto.String("mf3"),
		Type: dto.MetricType_UNTYPED.Enum(),
		Metric: []*dto.Metric{
			&dto.Metric{
				Label: []*dto.LabelPair{
					&dto.LabelPair{
						Name:  proto.String("job"),
						Value: proto.String("job1"),
					},
					&dto.LabelPair{
						Name:  proto.String("instance"),
						Value: proto.String("instance1"),
					},
				},
				Untyped: &dto.Untyped{
					Value: proto.Float64(42),
				},
			},
		},
	}
	mf4 = &dto.MetricFamily{
		Name: proto.String("mf4"),
		Type: dto.MetricType_UNTYPED.Enum(),
		Metric: []*dto.Metric{
			&dto.Metric{
				Label: []*dto.LabelPair{
					&dto.LabelPair{
						Name:  proto.String("job"),
						Value: proto.String("job3"),
					},
					&dto.LabelPair{
						Name:  proto.String("instance"),
						Value: proto.String("instance2"),
					},
				},
				Untyped: &dto.Untyped{
					Value: proto.Float64(3.4345),
				},
			},
		},
	}
)

func TestGetMetricFamilies(t *testing.T) {
	testTime := time.Now()
	j2i := JobToInstanceMap{
		"job1": InstanceToNameMap{
			"instance1": NameToTimestampedMetricFamilyMap{
				"mf1": TimestampedMetricFamily{
					Timestamp:    testTime,
					MetricFamily: mf2,
				},
			},
			"instance2": NameToTimestampedMetricFamilyMap{
				"mf1": TimestampedMetricFamily{
					Timestamp:    testTime,
					MetricFamily: mf1a,
				},
				"mf2": TimestampedMetricFamily{
					Timestamp:    testTime,
					MetricFamily: mf3,
				},
			},
		},
		"job2": InstanceToNameMap{},
		"job3": InstanceToNameMap{
			"instance1": NameToTimestampedMetricFamilyMap{},
			"instance2": NameToTimestampedMetricFamilyMap{
				"mf4": TimestampedMetricFamily{
					Timestamp:    testTime,
					MetricFamily: mf4,
				},
			},
		},
	}

	dms := &DiskMetricStore{metricFamilies: j2i}

	if err := checkMetricFamilies(dms, mf1a, mf2, mf3, mf4); err != nil {
		t.Error(err)
	}
}

func TestAddDeletePersistRestore(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "diskmetricstore.TestAddDeletePersistRestore.")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	fileName := path.Join(tempDir, "persistence")
	dms := NewDiskMetricStore(fileName, 100*time.Millisecond)

	// Submit a single simple metric family.
	ts1 := time.Now()
	dms.SubmitWriteRequest(WriteRequest{
		Job:            "job1",
		Instance:       "instance1",
		Timestamp:      ts1,
		MetricFamilies: map[string]*dto.MetricFamily{"mf3": mf3},
	})
	time.Sleep(time.Microsecond) // Give loop() time to process.
	if err := checkMetricFamilies(dms, mf3); err != nil {
		t.Error(err)
	}

	// Submit two metric families for a different instance.
	ts2 := ts1.Add(time.Second)
	dms.SubmitWriteRequest(WriteRequest{
		Job:            "job1",
		Instance:       "instance2",
		Timestamp:      ts2,
		MetricFamilies: map[string]*dto.MetricFamily{"mf1": mf1b, "mf2": mf2},
	})
	time.Sleep(time.Microsecond) // Give loop() time to process.
	if err := checkMetricFamilies(dms, mf1b, mf2, mf3); err != nil {
		t.Error(err)
	}

	// Submit a metric family with the same name for the same job/instance again.
	// Should overwrite the previous metric family for the same job/instance
	ts3 := ts2.Add(time.Second)
	dms.SubmitWriteRequest(WriteRequest{
		Job:            "job1",
		Instance:       "instance2",
		Timestamp:      ts3,
		MetricFamilies: map[string]*dto.MetricFamily{"mf1": mf1a},
	})
	time.Sleep(time.Microsecond) // Give loop() time to process.
	if err := checkMetricFamilies(dms, mf1a, mf2, mf3); err != nil {
		t.Error(err)
	}

	// Shutdown the dms.
	if err := dms.Shutdown(); err != nil {
		t.Fatal(err)
	}

	// Load it again.
	dms = NewDiskMetricStore(fileName, 100*time.Millisecond)
	if err := checkMetricFamilies(dms, mf1a, mf2, mf3); err != nil {
		t.Error(err)
	}
	// Spot-check timestamp.
	tmf := dms.metricFamilies["job1"]["instance2"]["mf1"]
	if expected, got := ts3, tmf.Timestamp; expected != got {
		t.Errorf("Expected timestamp %v, got %v.", expected, got)
	}

	// Delete an instance.
	dms.SubmitWriteRequest(WriteRequest{
		Job:      "job1",
		Instance: "instance1",
	})
	time.Sleep(time.Microsecond) // Give loop() time to process.
	if err := checkMetricFamilies(dms, mf1a, mf2); err != nil {
		t.Error(err)
	}

	// Submit another one.
	ts4 := ts3.Add(time.Second)
	dms.SubmitWriteRequest(WriteRequest{
		Job:            "job3",
		Instance:       "instance2",
		Timestamp:      ts4,
		MetricFamilies: map[string]*dto.MetricFamily{"mf4": mf4},
	})
	time.Sleep(time.Microsecond) // Give loop() time to process.
	if err := checkMetricFamilies(dms, mf1a, mf2, mf4); err != nil {
		t.Error(err)
	}

	// Delete a job.
	dms.SubmitWriteRequest(WriteRequest{
		Job: "job1",
	})
	time.Sleep(time.Microsecond) // Give loop() time to process.
	if err := checkMetricFamilies(dms, mf4); err != nil {
		t.Error(err)
	}

	// Delete last instance of a job.
	dms.SubmitWriteRequest(WriteRequest{
		Job:      "job3",
		Instance: "instance2",
	})
	time.Sleep(time.Microsecond) // Give loop() time to process.
	if err := checkMetricFamilies(dms); err != nil {
		t.Error(err)
	}
	// Check that no empty instance map for job3 was left behind.
	if _, stillExists := dms.metricFamilies["job3"]; stillExists {
		t.Error("An instance map for 'job3' still exists.")
	}

	// Delete a non existing job.
	dms.SubmitWriteRequest(WriteRequest{
		Job: "job4",
	})
	time.Sleep(150 * time.Millisecond) // Give time for persistence to kick in.
	if err := checkMetricFamilies(dms); err != nil {
		t.Error(err)
	}

	// Shutdown the dms again, directly after a number of write request
	// (to check draining).
	for i := 0; i < 10; i++ {
		dms.SubmitWriteRequest(WriteRequest{
			Job:            "job3",
			Instance:       "instance2",
			Timestamp:      ts4,
			MetricFamilies: map[string]*dto.MetricFamily{"mf4": mf4},
		})
	}
	if err := dms.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if err := checkMetricFamilies(dms, mf4); err != nil {
		t.Error(err)
	}
}

func checkNoPersistenc(t *testing.T) {
	dms := NewDiskMetricStore("", 100*time.Millisecond)

	ts1 := time.Now()
	dms.SubmitWriteRequest(WriteRequest{
		Job:            "job1",
		Instance:       "instance1",
		Timestamp:      ts1,
		MetricFamilies: map[string]*dto.MetricFamily{"mf3": mf3},
	})
	time.Sleep(time.Microsecond) // Give loop() time to process.
	if err := checkMetricFamilies(dms, mf3); err != nil {
		t.Error(err)
	}

	if err := dms.Shutdown(); err != nil {
		t.Fatal(err)
	}

	dms = NewDiskMetricStore("", 100*time.Millisecond)
	if err := checkMetricFamilies(dms); err != nil {
		t.Error(err)
	}
}

func checkMetricFamilies(dms *DiskMetricStore, expectedMFs ...*dto.MetricFamily) error {
	gotMFs := dms.GetMetricFamilies()
	if expected, got := len(expectedMFs), len(gotMFs); expected != got {
		return fmt.Errorf("expected %d metric families, got %d", expected, got)
	}

	expectedMFsAsStrings := make([]string, len(expectedMFs))
	for i, mf := range expectedMFs {
		expectedMFsAsStrings[i] = mf.String()
	}
	sort.Strings(expectedMFsAsStrings)

	gotMFsAsStrings := make([]string, len(gotMFs))
	for i, mf := range gotMFs {
		gotMFsAsStrings[i] = mf.String()
	}
	sort.Strings(gotMFsAsStrings)

	for i, got := range gotMFsAsStrings {
		expected := expectedMFsAsStrings[i]
		if expected != got {
			return fmt.Errorf("expected metric family %#v, got %#v", expected, got)
		}
	}
	return nil
}
