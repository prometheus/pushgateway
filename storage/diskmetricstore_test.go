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

	"github.com/go-kit/kit/log"

	"github.com/golang/protobuf/proto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	dto "github.com/prometheus/client_model/go"
)

var (
	logger = log.NewNopLogger()
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
	mf5 = &dto.MetricFamily{
		Name: proto.String("mf5"),
		Type: dto.MetricType_SUMMARY.Enum(),
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("job"),
						Value: proto.String("job5"),
					},
					{
						Name:  proto.String("instance"),
						Value: proto.String("instance5"),
					},
				},
				Summary: &dto.Summary{
					SampleCount: proto.Uint64(0),
					SampleSum:   proto.Float64(0),
				},
			},
		},
	}
	mfh1 = &dto.MetricFamily{
		Name: proto.String("mf_help"),
		Help: proto.String("Help string for mfh1."),
		Type: dto.MetricType_GAUGE.Enum(),
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("job"),
						Value: proto.String("job1"),
					},
				},
				Gauge: &dto.Gauge{
					Value: proto.Float64(3948.838),
				},
			},
		},
	}
	mfh2 = &dto.MetricFamily{
		Name: proto.String("mf_help"),
		Help: proto.String("Help string for mfh2."),
		Type: dto.MetricType_GAUGE.Enum(),
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("job"),
						Value: proto.String("job2"),
					},
				},
				Gauge: &dto.Gauge{
					Value: proto.Float64(83),
				},
			},
		},
	}
	// Both mfh metrics with mfh1's help string.
	mfh12 = &dto.MetricFamily{
		Name: proto.String("mf_help"),
		Help: proto.String("Help string for mfh1."),
		Type: dto.MetricType_GAUGE.Enum(),
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("job"),
						Value: proto.String("job1"),
					},
				},
				Gauge: &dto.Gauge{
					Value: proto.Float64(3948.838),
				},
			},
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("job"),
						Value: proto.String("job2"),
					},
				},
				Gauge: &dto.Gauge{
					Value: proto.Float64(83),
				},
			},
		},
	}
	// Both mfh metrics with mfh2's help string.
	mfh21 = &dto.MetricFamily{
		Name: proto.String("mf_help"),
		Help: proto.String("Help string for mfh2."),
		Type: dto.MetricType_GAUGE.Enum(),
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("job"),
						Value: proto.String("job1"),
					},
				},
				Gauge: &dto.Gauge{
					Value: proto.Float64(3948.838),
				},
			},
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("job"),
						Value: proto.String("job2"),
					},
				},
				Gauge: &dto.Gauge{
					Value: proto.Float64(83),
				},
			},
		},
	}
	mfgg = &dto.MetricFamily{
		Name: proto.String("go_goroutines"),
		Help: proto.String("Inconsistent doc string, fixed version in mfggFixed."),
		Type: dto.MetricType_GAUGE.Enum(),
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("job"),
						Value: proto.String("job1"),
					},
				},
				Gauge: &dto.Gauge{
					Value: proto.Float64(5),
				},
			},
		},
	}
	mfggFixed = &dto.MetricFamily{
		Name: proto.String("go_goroutines"),
		Help: proto.String("Number of goroutines that currently exist."),
		Type: dto.MetricType_GAUGE.Enum(),
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{
						Name:  proto.String("job"),
						Value: proto.String("job1"),
					},
				},
				Gauge: &dto.Gauge{
					Value: proto.Float64(5),
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
				Timestamp:            testTime,
				GobbableMetricFamily: (*GobbableMetricFamily)(mf2),
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
				Timestamp:            testTime,
				GobbableMetricFamily: (*GobbableMetricFamily)(mf1a),
			},
			"mf3": TimestampedMetricFamily{
				Timestamp:            testTime,
				GobbableMetricFamily: (*GobbableMetricFamily)(mf3),
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
				Timestamp:            testTime,
				GobbableMetricFamily: (*GobbableMetricFamily)(mf1c),
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
				Timestamp:            testTime,
				GobbableMetricFamily: (*GobbableMetricFamily)(mf4),
			},
			"mf1": TimestampedMetricFamily{
				Timestamp:            testTime,
				GobbableMetricFamily: (*GobbableMetricFamily)(mf1d),
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
	dms := NewDiskMetricStore(fileName, 100*time.Millisecond, nil, logger)

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

	// Add a new group by job, with a summary without any observations yet.
	ts4 := ts3.Add(time.Second)
	dms.SubmitWriteRequest(WriteRequest{
		Labels: map[string]string{
			"job": "job5",
		},
		Timestamp:      ts4,
		MetricFamilies: map[string]*dto.MetricFamily{"mf5": mf5},
	})
	time.Sleep(20 * time.Millisecond) // Give loop() time to process.
	if err := checkMetricFamilies(dms, mf1a, mf2, mf3, mf5); err != nil {
		t.Error(err)
	}

	// Shutdown the dms.
	if err := dms.Shutdown(); err != nil {
		t.Fatal(err)
	}

	// Load it again.
	dms = NewDiskMetricStore(fileName, 100*time.Millisecond, nil, logger)
	if err := checkMetricFamilies(dms, mf1a, mf2, mf3, mf5); err != nil {
		t.Error(err)
	}
	// Spot-check timestamp.
	tmf := dms.metricGroups[model.LabelsToSignature(map[string]string{
		"job":      "job1",
		"instance": "instance2",
	})].Metrics["mf1"]
	if expected, got := ts3, tmf.Timestamp; !expected.Equal(got) {
		t.Errorf("Expected timestamp %v, got %v.", expected, got)
	}

	// Delete two groups.
	dms.SubmitWriteRequest(WriteRequest{
		Labels: map[string]string{
			"job":      "job1",
			"instance": "instance1",
		},
	})
	dms.SubmitWriteRequest(WriteRequest{
		Labels: map[string]string{
			"job": "job5",
		},
	})
	time.Sleep(20 * time.Millisecond) // Give loop() time to process.
	if err := checkMetricFamilies(dms, mf1a, mf2); err != nil {
		t.Error(err)
	}

	// Submit another one.
	ts5 := ts4.Add(time.Second)
	dms.SubmitWriteRequest(WriteRequest{
		Labels: map[string]string{
			"job":      "job3",
			"instance": "instance2",
		},
		Timestamp:      ts5,
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
	// Check that no empty map entry for job3 was left behind.
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
	dms := NewDiskMetricStore("", 100*time.Millisecond, nil, logger)

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

	dms = NewDiskMetricStore("", 100*time.Millisecond, nil, logger)
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

func TestGetMetricFamiliesMap(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "diskmetricstore.TestGetMetricFamiliesMap.")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	fileName := path.Join(tempDir, "persistence")

	dms := NewDiskMetricStore(fileName, 100*time.Millisecond, nil, logger)

	labels1 := map[string]string{
		"job":      "job1",
		"instance": "instance1",
	}

	labels2 := map[string]string{
		"job":      "job2",
		"instance": "instance2",
	}

	ls1 := model.LabelsToSignature(labels1)
	ls2 := model.LabelsToSignature(labels2)

	// Submit a single simple metric family.
	ts1 := time.Now()
	dms.SubmitWriteRequest(WriteRequest{
		Labels:         labels1,
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
		Labels:         labels2,
		Timestamp:      ts2,
		MetricFamilies: map[string]*dto.MetricFamily{"mf1": mf1b, "mf2": mf2},
	})
	time.Sleep(20 * time.Millisecond) // Give loop() time to process.

	// expectedMFMap is a multi-layered map that maps the labelset fingerprints to the corresponding metric family string representations.
	// This is for test assertion purposes.
	expectedMFMap := map[uint64]map[string]string{
		ls1: {
			"mf3": mf3.String(),
		},
		ls2: {
			"mf1": mf1b.String(),
			"mf2": mf2.String(),
		},
	}

	if err := checkMetricFamilyGroups(dms, expectedMFMap); err != nil {
		t.Error(err)
	}
}

func TestHelpStringFix(t *testing.T) {
	dms := NewDiskMetricStore("", 100*time.Millisecond, prometheus.DefaultGatherer, logger)

	ts1 := time.Now()
	dms.SubmitWriteRequest(WriteRequest{
		Labels: map[string]string{
			"job": "job1",
		},
		Timestamp: ts1,
		MetricFamilies: map[string]*dto.MetricFamily{
			"go_goroutines": mfgg,
			"mf_help":       mfh1,
		},
	})
	dms.SubmitWriteRequest(WriteRequest{
		Labels: map[string]string{
			"job": "job2",
		},
		Timestamp: ts1,
		MetricFamilies: map[string]*dto.MetricFamily{
			"mf_help": mfh2,
		},
	})
	time.Sleep(20 * time.Millisecond) // Give loop() time to process.

	// Either we have settle on the mfh1 help string or the mfh2 help string.
	gotMFs := dms.GetMetricFamilies()
	if len(gotMFs) != 2 {
		t.Fatalf("expected 2 metric families, got %d", len(gotMFs))
	}
	gotMFsAsStrings := make([]string, len(gotMFs))
	for i, mf := range gotMFs {
		sort.Sort(metricSorter(mf.GetMetric()))
		gotMFsAsStrings[i] = mf.String()
	}
	sort.Strings(gotMFsAsStrings)
	gotGG := gotMFsAsStrings[0]
	got12 := gotMFsAsStrings[1]
	expectedGG := mfggFixed.String()
	expected12 := mfh12.String()
	expected21 := mfh21.String()

	if gotGG != expectedGG {
		t.Errorf(
			"help strings weren't properly adjusted, got '%s', expected '%s'",
			gotGG, expectedGG,
		)
	}
	if got12 != expected12 && got12 != expected21 {
		t.Errorf(
			"help strings weren't properly adjusted, got '%s' which is neither '%s' nor '%s'",
			got12, expected12, expected21,
		)
	}

	if err := dms.Shutdown(); err != nil {
		t.Fatal(err)
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
		sort.Sort(metricSorter(mf.GetMetric()))
		gotMFsAsStrings[i] = mf.String()
	}
	sort.Strings(gotMFsAsStrings)

	for i, got := range gotMFsAsStrings {
		expected := expectedMFsAsStrings[i]
		if expected != got {
			return fmt.Errorf("expected metric family '%s', got '%s'", expected, got)
		}
	}
	return nil
}

func checkMetricFamilyGroups(dms *DiskMetricStore, expectedMFMap map[uint64]map[string]string) error {
	mfMap := dms.GetMetricFamiliesMap()

	if expected, got := len(expectedMFMap), len(mfMap); expected != got {
		return fmt.Errorf("expected %d metric families in map, but got %d", expected, got)
	}

	for k, v := range mfMap {
		if innerMap, ok := expectedMFMap[k]; ok {
			if len(innerMap) != len(v.Metrics) {
				return fmt.Errorf("expected %d metric entries for labelSet fingerprint %d  in map, but got %d",
					len(innerMap), k, len(v.Metrics))
			}
			for metricName, metricString := range innerMap {
				if v.Metrics[metricName].GetMetricFamily().String() != metricString {
					return fmt.Errorf("expected metric %s to be present for key %s", metricString, metricName)
				}
			}
		} else {
			return fmt.Errorf("expected key value %d to be present in metric families in map", k)
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
