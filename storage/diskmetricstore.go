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
	"encoding/gob"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/golang/protobuf/proto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	dto "github.com/prometheus/client_model/go"
)

const (
	pushMetricName       = "push_time_seconds"
	pushMetricHelp       = "Last Unix time when changing this group in the Pushgateway succeeded."
	pushFailedMetricName = "push_failure_time_seconds"
	pushFailedMetricHelp = "Last Unix time when changing this group in the Pushgateway failed."
	writeQueueCapacity   = 1000
)

var errTimestamp = errors.New("pushed metrics must not have timestamps")

// DiskMetricStore is an implementation of MetricStore that persists metrics to
// disk.
type DiskMetricStore struct {
	lock            sync.RWMutex // Protects metricFamilies.
	writeQueue      chan WriteRequest
	drain           chan struct{}
	done            chan error
	metricGroups    GroupingKeyToMetricGroup
	persistenceFile string
	predefinedHelp  map[string]string
	logger          log.Logger
}

type mfStat struct {
	pos    int  // Where in the result slice is the MetricFamily?
	copied bool // Has the MetricFamily already been copied?
}

// NewDiskMetricStore returns a DiskMetricStore ready to use. To cleanly shut it
// down and free resources, the Shutdown() method has to be called.
//
// If persistenceFile is the empty string, no persisting to disk will
// happen. Otherwise, a file of that name is used for persisting metrics to
// disk. If the file already exists, metrics are read from it as part of the
// start-up. Persisting is happening upon shutdown and after every write action,
// but the latter will only happen persistenceDuration after the previous
// persisting.
//
// If a non-nil Gatherer is provided, the help strings of metrics gathered by it
// will be used as standard. Pushed metrics with deviating help strings will be
// adjusted to avoid inconsistent expositions.
func NewDiskMetricStore(
	persistenceFile string,
	persistenceInterval time.Duration,
	gatherPredefinedHelpFrom prometheus.Gatherer,
	logger log.Logger,
) *DiskMetricStore {
	// TODO: Do that outside of the constructor to allow the HTTP server to
	//  serve /-/healthy and /-/ready earlier.
	dms := &DiskMetricStore{
		writeQueue:      make(chan WriteRequest, writeQueueCapacity),
		drain:           make(chan struct{}),
		done:            make(chan error),
		metricGroups:    GroupingKeyToMetricGroup{},
		persistenceFile: persistenceFile,
		logger:          logger,
	}
	if err := dms.restore(); err != nil {
		level.Error(logger).Log("msg", "could not load persisted metrics", "err", err)
	}
	if helpStrings, err := extractPredefinedHelpStrings(gatherPredefinedHelpFrom); err == nil {
		dms.predefinedHelp = helpStrings
	} else {
		level.Error(logger).Log("msg", "could not gather metrics for predefined help strings", "err", err)
	}

	go dms.loop(persistenceInterval)
	return dms
}

// SubmitWriteRequest implements the MetricStore interface.
func (dms *DiskMetricStore) SubmitWriteRequest(req WriteRequest) {
	dms.writeQueue <- req
}

// Shutdown implements the MetricStore interface.
func (dms *DiskMetricStore) Shutdown() error {
	close(dms.drain)
	return <-dms.done
}

// Healthy implements the MetricStore interface.
func (dms *DiskMetricStore) Healthy() error {
	// By taking the lock we check that there is no deadlock.
	dms.lock.Lock()
	defer dms.lock.Unlock()

	// A pushgateway that cannot be written to should not be
	// considered as healthy.
	if len(dms.writeQueue) == cap(dms.writeQueue) {
		return fmt.Errorf("write queue is full")
	}

	return nil
}

// Ready implements the MetricStore interface.
func (dms *DiskMetricStore) Ready() error {
	return dms.Healthy()
}

// GetMetricFamilies implements the MetricStore interface.
func (dms *DiskMetricStore) GetMetricFamilies() []*dto.MetricFamily {
	dms.lock.RLock()
	defer dms.lock.RUnlock()

	result := []*dto.MetricFamily{}
	mfStatByName := map[string]mfStat{}

	for _, group := range dms.metricGroups {
		for name, tmf := range group.Metrics {
			mf := tmf.GetMetricFamily()
			if mf == nil {
				level.Warn(dms.logger).Log("msg", "storage corruption detected, consider wiping the persistence file")
				continue
			}
			stat, exists := mfStatByName[name]
			if exists {
				existingMF := result[stat.pos]
				if !stat.copied {
					mfStatByName[name] = mfStat{
						pos:    stat.pos,
						copied: true,
					}
					existingMF = copyMetricFamily(existingMF)
					result[stat.pos] = existingMF
				}
				if mf.GetHelp() != existingMF.GetHelp() {
					level.Info(dms.logger).Log("msg", "metric families inconsistent help strings", "err", "Metric families have inconsistent help strings. The latter will have priority. This is bad. Fix your pushed metrics!", "new", mf, "old", existingMF)
				}
				// Type inconsistency cannot be fixed here. We will detect it during
				// gathering anyway, so no reason to log anything here.
				existingMF.Metric = append(existingMF.Metric, mf.Metric...)
			} else {
				copied := false
				if help, ok := dms.predefinedHelp[name]; ok && mf.GetHelp() != help {
					level.Info(dms.logger).Log("msg", "metric families overlap", "err", "Metric family has the same name as a metric family used by the Pushgateway itself but it has a different help string. Changing it to the standard help string. This is bad. Fix your pushed metrics!", "metric_family", mf, "standard_help", help)
					mf = copyMetricFamily(mf)
					copied = true
					mf.Help = proto.String(help)
				}
				mfStatByName[name] = mfStat{
					pos:    len(result),
					copied: copied,
				}
				result = append(result, mf)
			}
		}
	}
	return result
}

// GetMetricFamiliesMap implements the MetricStore interface.
func (dms *DiskMetricStore) GetMetricFamiliesMap() GroupingKeyToMetricGroup {
	dms.lock.RLock()
	defer dms.lock.RUnlock()
	groupsCopy := make(GroupingKeyToMetricGroup, len(dms.metricGroups))
	for k, g := range dms.metricGroups {
		metricsCopy := make(NameToTimestampedMetricFamilyMap, len(g.Metrics))
		groupsCopy[k] = MetricGroup{Labels: g.Labels, Metrics: metricsCopy}
		for n, tmf := range g.Metrics {
			metricsCopy[n] = tmf
		}
	}
	return groupsCopy
}

func (dms *DiskMetricStore) loop(persistenceInterval time.Duration) {
	lastPersist := time.Now()
	persistScheduled := false
	lastWrite := time.Time{}
	persistDone := make(chan time.Time)
	var persistTimer *time.Timer

	checkPersist := func() {
		if dms.persistenceFile != "" && !persistScheduled && lastWrite.After(lastPersist) {
			persistTimer = time.AfterFunc(
				persistenceInterval-lastWrite.Sub(lastPersist),
				func() {
					persistStarted := time.Now()
					if err := dms.persist(); err != nil {
						level.Error(dms.logger).Log("msg", "error persisting metrics", "err", err)
					} else {
						level.Info(dms.logger).Log("msg", "metrics persisted", "file", dms.persistenceFile)
					}
					persistDone <- persistStarted
				},
			)
			persistScheduled = true
		}
	}

	for {
		select {
		case wr := <-dms.writeQueue:
			lastWrite = time.Now()
			if dms.checkWriteRequest(wr) {
				dms.processWriteRequest(wr)
			} else {
				dms.setPushFailedTimestamp(wr)
			}
			if wr.Done != nil {
				close(wr.Done)
			}
			checkPersist()
		case lastPersist = <-persistDone:
			persistScheduled = false
			checkPersist() // In case something has been written in the meantime.
		case <-dms.drain:
			// Prevent a scheduled persist from firing later.
			if persistTimer != nil {
				persistTimer.Stop()
			}
			// Now draining...
			for {
				select {
				case wr := <-dms.writeQueue:
					dms.processWriteRequest(wr)
				default:
					dms.done <- dms.persist()
					return
				}
			}
		}
	}
}

func (dms *DiskMetricStore) processWriteRequest(wr WriteRequest) {
	dms.lock.Lock()
	defer dms.lock.Unlock()

	key := groupingKeyFor(wr.Labels)

	if wr.MetricFamilies == nil {
		// No MetricFamilies means delete request. Delete the whole
		// metric group, and we are done here.
		delete(dms.metricGroups, key)
		return
	}
	// Otherwise, it's an update.
	group, ok := dms.metricGroups[key]
	if !ok {
		group = MetricGroup{
			Labels:  wr.Labels,
			Metrics: NameToTimestampedMetricFamilyMap{},
		}
		dms.metricGroups[key] = group
	} else if wr.Replace {
		// For replace, we have to delete all metric families in the
		// group except pre-existing push timestamps.
		for name := range group.Metrics {
			if name != pushMetricName && name != pushFailedMetricName {
				delete(group.Metrics, name)
			}
		}
	}
	wr.MetricFamilies[pushMetricName] = newPushTimestampGauge(wr.Labels, wr.Timestamp)
	// Only add a zero push-failed metric if none is there yet, so that a
	// previously added fail timestamp is retained.
	if _, ok := group.Metrics[pushFailedMetricName]; !ok {
		wr.MetricFamilies[pushFailedMetricName] = newPushFailedTimestampGauge(wr.Labels, time.Time{})
	}
	for name, mf := range wr.MetricFamilies {
		group.Metrics[name] = TimestampedMetricFamily{
			Timestamp:            wr.Timestamp,
			GobbableMetricFamily: (*GobbableMetricFamily)(mf),
		}
	}
}

func (dms *DiskMetricStore) setPushFailedTimestamp(wr WriteRequest) {
	dms.lock.Lock()
	defer dms.lock.Unlock()

	key := groupingKeyFor(wr.Labels)

	group, ok := dms.metricGroups[key]
	if !ok {
		group = MetricGroup{
			Labels:  wr.Labels,
			Metrics: NameToTimestampedMetricFamilyMap{},
		}
		dms.metricGroups[key] = group
	}

	group.Metrics[pushFailedMetricName] = TimestampedMetricFamily{
		Timestamp:            wr.Timestamp,
		GobbableMetricFamily: (*GobbableMetricFamily)(newPushFailedTimestampGauge(wr.Labels, wr.Timestamp)),
	}
	// Only add a zero push metric if none is there yet, so that a
	// previously added push timestamp is retained.
	if _, ok := group.Metrics[pushMetricName]; !ok {
		group.Metrics[pushMetricName] = TimestampedMetricFamily{
			Timestamp:            wr.Timestamp,
			GobbableMetricFamily: (*GobbableMetricFamily)(newPushTimestampGauge(wr.Labels, time.Time{})),
		}
	}
}

// checkWriteRequest return if applying the provided WriteRequest will result in
// a consistent state of metrics. The dms is not modified by the check. However,
// the WriteRequest _will_ be sanitized: the MetricFamilies are ensured to
// contain the grouping Labels after the check. If false is returned, the
// causing error is written to the Done channel of the WriteRequest.
//
// Special case: If the WriteRequest has no Done channel set, the (expensive)
// consistency check is skipped. The WriteRequest is still sanitized, and the
// presence of timestamps still results in returning false.
func (dms *DiskMetricStore) checkWriteRequest(wr WriteRequest) bool {
	if wr.MetricFamilies == nil {
		// Delete request cannot create inconsistencies, and nothing has
		// to be sanitized.
		return true
	}

	var err error
	defer func() {
		if err != nil && wr.Done != nil {
			wr.Done <- err
		}
	}()

	if timestampsPresent(wr.MetricFamilies) {
		err = errTimestamp
		return false
	}
	for _, mf := range wr.MetricFamilies {
		sanitizeLabels(mf, wr.Labels)
	}

	// Without Done channel, don't do the expensive consistency check.
	if wr.Done == nil {
		return true
	}

	// Construct a test dms, acting on a copy of the metrics, to test the
	// WriteRequest with.
	tdms := &DiskMetricStore{
		metricGroups:   dms.GetMetricFamiliesMap(),
		predefinedHelp: dms.predefinedHelp,
		logger:         log.NewNopLogger(),
	}
	tdms.processWriteRequest(wr)

	// Construct a test Gatherer to check if consistent gathering is possible.
	tg := prometheus.Gatherers{
		prometheus.DefaultGatherer,
		prometheus.GathererFunc(func() ([]*dto.MetricFamily, error) {
			return tdms.GetMetricFamilies(), nil
		}),
	}
	if _, err = tg.Gather(); err != nil {
		return false
	}
	return true
}

func (dms *DiskMetricStore) persist() error {
	// Check (again) if persistence is configured because some code paths
	// will call this method even if it is not.
	if dms.persistenceFile == "" {
		return nil
	}
	f, err := ioutil.TempFile(
		path.Dir(dms.persistenceFile),
		path.Base(dms.persistenceFile)+".in_progress.",
	)
	if err != nil {
		return err
	}
	inProgressFileName := f.Name()
	e := gob.NewEncoder(f)

	dms.lock.RLock()
	err = e.Encode(dms.metricGroups)
	dms.lock.RUnlock()
	if err != nil {
		f.Close()
		os.Remove(inProgressFileName)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(inProgressFileName)
		return err
	}
	return os.Rename(inProgressFileName, dms.persistenceFile)
}

func (dms *DiskMetricStore) restore() error {
	if dms.persistenceFile == "" {
		return nil
	}
	f, err := os.Open(dms.persistenceFile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	d := gob.NewDecoder(f)
	if err := d.Decode(&dms.metricGroups); err != nil {
		return err
	}
	return nil
}

func copyMetricFamily(mf *dto.MetricFamily) *dto.MetricFamily {
	return &dto.MetricFamily{
		Name:   mf.Name,
		Help:   mf.Help,
		Type:   mf.Type,
		Metric: append([]*dto.Metric{}, mf.Metric...),
	}
}

// groupingKeyFor creates a grouping key from the provided map of grouping
// labels. The grouping key is created by joining all label names and values
// together with model.SeparatorByte as a separator. The label names are sorted
// lexicographically before joining. In that way, the grouping key is both
// reproducible and unique.
func groupingKeyFor(labels map[string]string) string {
	if len(labels) == 0 { // Super fast path.
		return ""
	}

	labelNames := make([]string, 0, len(labels))
	for labelName := range labels {
		labelNames = append(labelNames, labelName)
	}
	sort.Strings(labelNames)

	sb := strings.Builder{}
	for i, labelName := range labelNames {
		sb.WriteString(labelName)
		sb.WriteByte(model.SeparatorByte)
		sb.WriteString(labels[labelName])
		if i+1 < len(labels) { // No separator at the end.
			sb.WriteByte(model.SeparatorByte)
		}
	}
	return sb.String()
}

// extractPredefinedHelpStrings extracts all the HELP strings from the provided
// gatherer so that the DiskMetricStore can fix deviations in pushed metrics.
func extractPredefinedHelpStrings(g prometheus.Gatherer) (map[string]string, error) {
	if g == nil {
		return nil, nil
	}
	mfs, err := g.Gather()
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, mf := range mfs {
		result[mf.GetName()] = mf.GetHelp()
	}
	return result, nil
}

func newPushTimestampGauge(groupingLabels map[string]string, t time.Time) *dto.MetricFamily {
	return newTimestampGauge(pushMetricName, pushMetricHelp, groupingLabels, t)
}

func newPushFailedTimestampGauge(groupingLabels map[string]string, t time.Time) *dto.MetricFamily {
	return newTimestampGauge(pushFailedMetricName, pushFailedMetricHelp, groupingLabels, t)
}

func newTimestampGauge(name, help string, groupingLabels map[string]string, t time.Time) *dto.MetricFamily {
	var ts float64
	if !t.IsZero() {
		ts = float64(t.UnixNano()) / 1e9
	}
	mf := &dto.MetricFamily{
		Name: proto.String(name),
		Help: proto.String(help),
		Type: dto.MetricType_GAUGE.Enum(),
		Metric: []*dto.Metric{
			{
				Gauge: &dto.Gauge{
					Value: proto.Float64(ts),
				},
			},
		},
	}
	sanitizeLabels(mf, groupingLabels)
	return mf
}

// sanitizeLabels ensures that all the labels in groupingLabels and the
// `instance` label are present in the MetricFamily. The label values from
// groupingLabels are set in each Metric, no matter what. After that, if the
// 'instance' label is not present at all in a Metric, it will be created (with
// an empty string as value).
//
// Finally, sanitizeLabels sorts the label pairs of all metrics.
func sanitizeLabels(mf *dto.MetricFamily, groupingLabels map[string]string) {
	gLabelsNotYetDone := make(map[string]string, len(groupingLabels))

metric:
	for _, m := range mf.GetMetric() {
		for ln, lv := range groupingLabels {
			gLabelsNotYetDone[ln] = lv
		}
		hasInstanceLabel := false
		for _, lp := range m.GetLabel() {
			ln := lp.GetName()
			if lv, ok := gLabelsNotYetDone[ln]; ok {
				lp.Value = proto.String(lv)
				delete(gLabelsNotYetDone, ln)
			}
			if ln == string(model.InstanceLabel) {
				hasInstanceLabel = true
			}
			if len(gLabelsNotYetDone) == 0 && hasInstanceLabel {
				sort.Sort(labelPairs(m.Label))
				continue metric
			}
		}
		for ln, lv := range gLabelsNotYetDone {
			m.Label = append(m.Label, &dto.LabelPair{
				Name:  proto.String(ln),
				Value: proto.String(lv),
			})
			if ln == string(model.InstanceLabel) {
				hasInstanceLabel = true
			}
			delete(gLabelsNotYetDone, ln) // To prepare map for next metric.
		}
		if !hasInstanceLabel {
			m.Label = append(m.Label, &dto.LabelPair{
				Name:  proto.String(string(model.InstanceLabel)),
				Value: proto.String(""),
			})
		}
		sort.Sort(labelPairs(m.Label))
	}
}

// Checks if any timestamps have been specified.
func timestampsPresent(metricFamilies map[string]*dto.MetricFamily) bool {
	for _, mf := range metricFamilies {
		for _, m := range mf.GetMetric() {
			if m.TimestampMs != nil {
				return true
			}
		}
	}
	return false
}

// labelPairs implements sort.Interface. It provides a sortable version of a
// slice of dto.LabelPair pointers.
type labelPairs []*dto.LabelPair

func (s labelPairs) Len() int {
	return len(s)
}

func (s labelPairs) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s labelPairs) Less(i, j int) bool {
	return s[i].GetName() < s[j].GetName()
}
