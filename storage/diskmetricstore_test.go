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

	"github.com/golang/protobuf/proto"
	"github.com/prometheus/common/model"

	dto "github.com/prometheus/client_model/go"
)

var (
	// Example metric families.
	mf1a = &dto.MetricFamily{
		Name: proto.String("mf1"),
		Type: dto.MetricType_UNTYPED.Enum(),
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("job"),
						Value: proto.String("job1"),
					},
					{
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
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("job"),
						Value: proto.String("job1"),
					},
					{
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
	mf1c = &dto.MetricFamily{
		Name: proto.String("mf1"),
		Type: dto.MetricType_UNTYPED.Enum(),
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("job"),
						Value: proto.String("job2"),
					},
					{
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
	mf1d = &dto.MetricFamily{
		Name: proto.String("mf1"),
		Type: dto.MetricType_UNTYPED.Enum(),
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("job"),
						Value: proto.String("job3"),
					},
					{
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
	// mf1acd is merged from mf1a, mf1c, mf1d.
	mf1acd = &dto.MetricFamily{
		Name: proto.String("mf1"),
		Type: dto.MetricType_UNTYPED.Enum(),
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("job"),
						Value: proto.String("job1"),
					},
					{
						Name:  proto.String("instance"),
						Value: proto.String("instance2"),
					},
				},
				Untyped: &dto.Untyped{
					Value: proto.Float64(-3e3),
				},
				TimestampMs: proto.Int64(103948),
			},
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("job"),
						Value: proto.String("job2"),
					},
					{
						Name:  proto.String("instance"),
						Value: proto.String("instance1"),
					},
				},
				Untyped: &dto.Untyped{
					Value: proto.Float64(42),
				},
			},
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("job"),
						Value: proto.String("job3"),
					},
					{
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
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("job"),
						Value: proto.String("job1"),
					},
					{
						Name:  proto.String("instance"),
						Value: proto.String("instance2"),
					},
					{
						Name:  proto.String("labelname"),
						Value: proto.String("val2"),
					},
					{
						Name:  proto.String("basename"),
						Value: proto.String("basevalue2"),
					},
				},
				Gauge: &dto.Gauge{
					Value: proto.Float64(math.Inf(+1)),
				},
				TimestampMs: proto.Int64(54321),
			},
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("job"),
						Value: proto.String("job1"),
					},
					{
						Name:  proto.String("instance"),
						Value: proto.String("instance2"),
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
	mf3 = &dto.MetricFamily{
		Name: proto.String("mf3"),
		Type: dto.MetricType_UNTYPED.Enum(),
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("job"),
						Value: proto.String("job1"),
					},
					{
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
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("job"),
						Value: proto.String("job3"),
					},
					{
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

func addGroup(
	mg GroupingKeyToMetricGroup,
	groupingLabels map[string]string,
	metrics NameToTimestampedMetricFamilyMap,
) {
	mg[model.LabelsToSignature(groupingLabels)] = MetricGroup{
		Labels:  groupingLabels,
		Metrics: metrics,
	}
}

func TestGetMetricFamilies(t *testing.T) {
	testTime := time.Now()

	mg := GroupingKeyToMetricGroup{}
	addGroup(
		mg,
		map[string]string{
			"job":      "job1",
			"instance": "instance1",
		},
		NameToTimestampedMetricFamilyMap{
			"mf2": TimestampedMetricFamily{
				Timestamp:    testTime,
				MetricFamily: mf2,
			},
		},
	)
	addGroup(
		mg,
		map[string]string{
			"job":      "job1",
			"instance": "instance2",
		},
		NameToTimestampedMetricFamilyMap{
			"mf1": TimestampedMetricFamily{
				Timestamp:    testTime,
				MetricFamily: mf1a,
			},
			"mf3": TimestampedMetricFamily{
				Timestamp:    testTime,
				MetricFamily: mf3,
			},
		},
	)
	addGroup(
		mg,
		map[string]string{
			"job":      "job2",
			"instance": "instance1",
		},
		NameToTimestampedMetricFamilyMap{
			"mf1": TimestampedMetricFamily{
				Timestamp:    testTime,
				MetricFamily: mf1c,
			},
		},
	)
	addGroup(
		mg,
		map[string]string{
			"job":      "job3",
			"instance": "instance1",
		},
		NameToTimestampedMetricFamilyMap{},
	)
	addGroup(
		mg,
		map[string]string{
			"job":      "job3",
			"instance": "instance2",
		},
		NameToTimestampedMetricFamilyMap{
			"mf4": TimestampedMetricFamily{
				Timestamp:    testTime,
				MetricFamily: mf4,
			},
			"mf1": TimestampedMetricFamily{
				Timestamp:    testTime,
				MetricFamily: mf1d,
			},
		},
	)
	addGroup(
		mg,
		map[string]string{
			"job": "job4",
		},
		NameToTimestampedMetricFamilyMap{},
	)

	dms := &DiskMetricStore{metricGroups: mg}

	if err := checkMetricFamilies(dms, mf1acd, mf2, mf3, mf4); err != nil {
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
		Labels: map[string]string{
			"job":      "job1",
			"instance": "instance1",
		},
		Timestamp:      ts1,
		MetricFamilies: map[string]*dto.MetricFamily{"mf3": mf3},
	})
	time.Sleep(20 * time.Millisecond) // Give loop() time to process.
	if err := checkMetricFamilies(dms, mf3); err != nil {
		t.Error(err)
	}

	// Submit two metric families for a different instance.
	ts2 := ts1.Add(time.Second)
	dms.SubmitWriteRequest(WriteRequest{
		Labels: map[string]string{
			"job":      "job1",
			"instance": "instance2",
		},
		Timestamp:      ts2,
		MetricFamilies: map[string]*dto.MetricFamily{"mf1": mf1b, "mf2": mf2},
	})
	time.Sleep(20 * time.Millisecond) // Give loop() time to process.
	if err := checkMetricFamilies(dms, mf1b, mf2, mf3); err != nil {
		t.Error(err)
	}

	// Submit a metric family with the same name for the same job/instance again.
	// Should overwrite the previous metric family for the same job/instance
	ts3 := ts2.Add(time.Second)
	dms.SubmitWriteRequest(WriteRequest{
		Labels: map[string]string{
			"job":      "job1",
			"instance": "instance2",
		},
		Timestamp:      ts3,
		MetricFamilies: map[string]*dto.MetricFamily{"mf1": mf1a},
	})
	time.Sleep(20 * time.Millisecond) // Give loop() time to process.
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
	tmf := dms.metricGroups[model.LabelsToSignature(map[string]string{
		"job":      "job1",
		"instance": "instance2",
	})].Metrics["mf1"]
	if expected, got := ts3, tmf.Timestamp; got.Sub(expected) != 0 {
		t.Errorf("Expected timestamp %v, got %v.", expected, got)
	}

	// Delete a group.
	dms.SubmitWriteRequest(WriteRequest{
		Labels: map[string]string{
			"job":      "job1",
			"instance": "instance1",
		},
	})
	time.Sleep(20 * time.Millisecond) // Give loop() time to process.
	if err := checkMetricFamilies(dms, mf1a, mf2); err != nil {
		t.Error(err)
	}

	// Submit another one.
	ts4 := ts3.Add(time.Second)
	dms.SubmitWriteRequest(WriteRequest{
		Labels: map[string]string{
			"job":      "job3",
			"instance": "instance2",
		},
		Timestamp:      ts4,
		MetricFamilies: map[string]*dto.MetricFamily{"mf4": mf4},
	})
	time.Sleep(20 * time.Millisecond) // Give loop() time to process.
	if err := checkMetricFamilies(dms, mf1a, mf2, mf4); err != nil {
		t.Error(err)
	}

	// Delete a job does not remove anything because there is no suitable
	// grouping.
	dms.SubmitWriteRequest(WriteRequest{
		Labels: map[string]string{
			"job": "job1",
		},
	})
	time.Sleep(20 * time.Millisecond) // Give loop() time to process.
	if err := checkMetricFamilies(dms, mf1a, mf2, mf4); err != nil {
		t.Error(err)
	}

	// Delete another group.
	dms.SubmitWriteRequest(WriteRequest{
		Labels: map[string]string{
			"job":      "job3",
			"instance": "instance2",
		},
	})
	time.Sleep(20 * time.Millisecond) // Give loop() time to process.
	if err := checkMetricFamilies(dms, mf1a, mf2); err != nil {
		t.Error(err)
	}
	// Check that no empty  map entry for job3 was left behind.
	if _, stillExists := dms.metricGroups[model.LabelsToSignature(map[string]string{
		"job":      "job3",
		"instance": "instance2",
	})]; stillExists {
		t.Error("An instance map for 'job3' still exists.")
	}

	// Shutdown the dms again, directly after a number of write request
	// (to check draining).
	for i := 0; i < 10; i++ {
		dms.SubmitWriteRequest(WriteRequest{
			Labels: map[string]string{
				"job":      "job3",
				"instance": "instance2",
			},
			Timestamp:      ts4,
			MetricFamilies: map[string]*dto.MetricFamily{"mf4": mf4},
		})
	}
	if err := dms.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if err := checkMetricFamilies(dms, mf1a, mf2, mf4); err != nil {
		t.Error(err)
	}
}

func TestNoPersistence(t *testing.T) {
	dms := NewDiskMetricStore("", 100*time.Millisecond)

	ts1 := time.Now()
	dms.SubmitWriteRequest(WriteRequest{
		Labels: map[string]string{
			"job":      "job1",
			"instance": "instance1",
		},
		Timestamp:      ts1,
		MetricFamilies: map[string]*dto.MetricFamily{"mf3": mf3},
	})
	time.Sleep(20 * time.Millisecond) // Give loop() time to process.
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

	if err := dms.Ready(); err != nil {
		t.Error(err)
	}

	if err := dms.Healthy(); err != nil {
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
		sort.Sort(metricSorter(mf.Metric))
		expectedMFsAsStrings[i] = mf.String()
	}
	sort.Strings(expectedMFsAsStrings)

	gotMFsAsStrings := make([]string, len(gotMFs))
	for i, mf := range gotMFs {
		sort.Sort(metricSorter(mf.Metric))
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

type metricSorter []*dto.Metric

func (s metricSorter) Len() int {
	return len(s)
}

func (s metricSorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s metricSorter) Less(i, j int) bool {
	for n, lp := range s[i].Label {
		vi := lp.GetValue()
		vj := s[j].Label[n].GetValue()
		if vi != vj {
			return vi < vj
		}
	}
	return true
}
