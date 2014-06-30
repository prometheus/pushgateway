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
	"time"

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
	// If different jobs and instances have saved MetricFamilies of the same
	// name, they are all merged into one MetricFamily by concatenating the
	// contained Metrics. Inconsistent help strings or types are logged, and
	// one of the versions will "win". Inconsistent labels will go
	// undetected.
	GetMetricFamilies() []*dto.MetricFamily
	// GetMetricFamiliesMap returns a nested map (job -> instance ->
	// metric-name -> TimestampedMetricFamily). The MetricFamily pointed to
	// by each TimestampedMetricFamily is guaranteed to not be modified by
	// the MetricStore anymore. However, they may still be read somewhere
	// else, so the caller is not allowed to modify it. Otherwise, the
	// returned nested map is a deep copy of the internal state of the
	// MetricStore and completely owned by the caller.
	GetMetricFamiliesMap() JobToInstanceMap
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
}

// WriteRequest is a request to change the MetricStore, i.e. to process it, a
// write lock has to be acquired. If MetricFamilies is nil, this is a request to
// delete metrics that share the given Job and (if not empty) Instance
// labels. Otherwise, this is a request to update the MetricStore with the
// MetricFamilies. The key in MetricFamilies is the name of the mapped metric
// family. All metrics in MetricFamilies MUST have already set job and instance
// labels that are consistent with the Job and Instance fields. The Timestamp
// field marks the time the request was received from the network. It is not
// related to the timestamp_ms field in the Metric proto message.
type WriteRequest struct {
	Job, Instance  string
	Timestamp      time.Time
	MetricFamilies map[string]*dto.MetricFamily
}

// TimestampedMetricFamily adds a timestamp to a MetricFamily-DTO.
type TimestampedMetricFamily struct {
	Timestamp    time.Time
	MetricFamily *dto.MetricFamily
}

// JobToInstanceMap is the first level of the metric store, keyed by job name.
type JobToInstanceMap map[string]InstanceToNameMap

// InstanceToNameMap is the second level of the metric store, keyed by instance
// name.
type InstanceToNameMap map[string]NameToTimestampedMetricFamilyMap

// NameToTimestampedMetricFamilyMap is the third level of the metric store,
// keyed by metric name.
type NameToTimestampedMetricFamilyMap map[string]TimestampedMetricFamily
