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
	"sort"
	"time"

	"github.com/golang/protobuf/proto"

	dto "github.com/prometheus/client_model/go"
)

// MetricStore is the interface to the storage layer for metrics. All its
// methods must be safe to be called concurrently.
type MetricStore interface {
	// SubmitWriteRequest submits a WriteRequest for processing. There is no
	// guarantee when a request will be processed, but it is guaranteed that
	// the requests are processed in the order of submission.
	SubmitWriteRequest(req WriteRequest)
	// GetMetricFamilies returns all the currently saved MetricFamilies. The
	// returned MetricFamilies are guaranteed to not be modified by the
	// MetricStore anymore. However, they may still be read somewhere else,
	// so the caller is not allowed to modify the returned MetricFamilies.
	// If different groups have saved MetricFamilies of the same name, they
	// are all merged into one MetricFamily by concatenating the contained
	// Metrics. Inconsistent help strings are logged, and one of the
	// versions will "win". Inconsistent types and inconsistent or duplicate
	// label sets will go undetected.
	GetMetricFamilies() []*dto.MetricFamily
	// GetMetricFamiliesMap returns a map grouping-key -> MetricGroup. The
	// MetricFamily pointed to by the Metrics map in each MetricGroup is
	// guaranteed to not be modified by the MetricStore anymore. However,
	// they may still be read somewhere else, so the caller is not allowed
	// to modify it. Otherwise, the returned nested map can be seen as a
	// deep copy of the internal state of the MetricStore and completely
	// owned by the caller.
	GetMetricFamiliesMap() GroupingKeyToMetricGroup
	// Shutdown must only be called after the caller has made sure that
	// SubmitWriteRequests is not called anymore. (If it is called later,
	// the request might get submitted, but not processed anymore.) The
	// Shutdown method waits for the write request queue to empty, then it
	// persists the content of the MetricStore (if supported by the
	// implementation). Also, all internal goroutines are stopped. This
	// method blocks until all of that is complete. If an error is
	// encountered, it is returned (whereupon the MetricStorage is in an
	// undefinded state). If nil is returned, the MetricStore cannot be
	// "restarted" again, but it can still be used for read operations.
	Shutdown() error
	// Healthy returns nil if the MetricStore is currently working as
	// expected. Otherwise, a non-nil error is returned.
	Healthy() error
	// Ready returns nil if the MetricStore is ready to be used (all files
	// are opened and checkpoints have been restored). Otherwise, a non-nil
	// error is returned.
	Ready() error
}

// WriteRequest is a request to change the MetricStore, i.e. to process it, a
// write lock has to be acquired.
//
// If MetricFamilies is nil, this is a request to delete metrics that share the
// given Labels as a grouping key. Otherwise, this is a request to update the
// MetricStore with the MetricFamilies.
//
// If Replace is true, the MetricFamilies will completely replace the metrics
// with the same grouping key. Otherwise, only those MetricFamilies with the
// same name as new MetricFamilies will be replaced.
//
// The key in MetricFamilies is the name of the mapped metric family.
//
// When the WriteRequest is processed, the metrics in MetricFamilies will be
// sanitized to have the same job and other labels as those in the Labels
// fields. Also, if there is no instance label, an instance label with an empty
// value will be set. This implies that the MetricFamilies in the WriteRequest
// may be modified be the MetricStore during processing of the WriteRequest!
//
// The Timestamp field marks the time the request was received from the
// network. It is not related to the TimestampMs field in the Metric proto
// message. In fact, WriteRequests containing any Metrics with a TimestampMs set
// are invalid and will be rejected.
//
// The Done channel may be nil. If it is not nil, it will be closed once the
// write request is processed. Any errors occurring during processing are sent to
// the channel before closing it.
type WriteRequest struct {
	Labels         map[string]string
	Timestamp      time.Time
	MetricFamilies map[string]*dto.MetricFamily
	Replace        bool
	Done           chan error
}

// GroupingKeyToMetricGroup is the first level of the metric store, keyed by
// grouping key.
type GroupingKeyToMetricGroup map[string]MetricGroup

// MetricGroup adds the grouping labels to a NameToTimestampedMetricFamilyMap.
type MetricGroup struct {
	Labels  map[string]string
	Metrics NameToTimestampedMetricFamilyMap
}

// SortedLabels returns the label names of the grouping labels sorted
// lexicographically but with the "job" label always first. This method exists
// for presentation purposes, see template.html.
func (mg MetricGroup) SortedLabels() []string {
	lns := make([]string, 1, len(mg.Labels))
	lns[0] = "job"
	for ln := range mg.Labels {
		if ln != "job" {
			lns = append(lns, ln)
		}
	}
	sort.Strings(lns[1:])
	return lns
}

// LastPushSuccess returns false if the automatically added metric for the
// timestamp of the last failed push has a value larger than the value of the
// automatically added metric for the timestamp of the last successful push. In
// all other cases, it returns true (including the case that one or both of
// those metrics are missing for some reason.)
func (mg MetricGroup) LastPushSuccess() bool {
	fail := mg.Metrics[pushFailedMetricName].GobbableMetricFamily
	if fail == nil {
		return true
	}
	success := mg.Metrics[pushMetricName].GobbableMetricFamily
	if success == nil {
		return true
	}
	return (*dto.MetricFamily)(fail).GetMetric()[0].GetGauge().GetValue() <= (*dto.MetricFamily)(success).GetMetric()[0].GetGauge().GetValue()
}

// NameToTimestampedMetricFamilyMap is the second level of the metric store,
// keyed by metric name.
type NameToTimestampedMetricFamilyMap map[string]TimestampedMetricFamily

// TimestampedMetricFamily adds the push timestamp to a gobbable version of the
// MetricFamily-DTO.
type TimestampedMetricFamily struct {
	Timestamp            time.Time
	GobbableMetricFamily *GobbableMetricFamily
}

// GetMetricFamily returns the normal GetMetricFamily DTO (without the gob additions).
func (tmf TimestampedMetricFamily) GetMetricFamily() *dto.MetricFamily {
	return (*dto.MetricFamily)(tmf.GobbableMetricFamily)
}

// GobbableMetricFamily is a dto.MetricFamily that implements GobDecoder and
// GobEncoder.
type GobbableMetricFamily dto.MetricFamily

// GobDecode implements gob.GobDecoder.
func (gmf *GobbableMetricFamily) GobDecode(b []byte) error {
	return proto.Unmarshal(b, (*dto.MetricFamily)(gmf))
}

// GobEncode implements gob.GobEncoder.
func (gmf *GobbableMetricFamily) GobEncode() ([]byte, error) {
	return proto.Marshal((*dto.MetricFamily)(gmf))
}
