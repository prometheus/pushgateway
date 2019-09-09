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
	"fmt"
	"io/ioutil"
	"os"
	"path"
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
	writeQueueCapacity = 1000
)

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

// GetMetricFamilies implements the MetricStore interface.
func (dms *DiskMetricStore) GetMetricFamilies() []*dto.MetricFamily {
	result := []*dto.MetricFamily{}
	mfStatByName := map[string]mfStat{}

	dms.lock.RLock()
	defer dms.lock.RUnlock()

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
			dms.processWriteRequest(wr)
			lastWrite = time.Now()
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

	key := model.LabelsToSignature(wr.Labels)

	if wr.MetricFamilies == nil {
		// Delete.
		delete(dms.metricGroups, key)
		return
	}
	// Update.
	for name, mf := range wr.MetricFamilies {
		group, ok := dms.metricGroups[key]
		if !ok {
			group = MetricGroup{
				Labels:  wr.Labels,
				Metrics: NameToTimestampedMetricFamilyMap{},
			}
			dms.metricGroups[key] = group
		}
		group.Metrics[name] = TimestampedMetricFamily{
			Timestamp:            wr.Timestamp,
			GobbableMetricFamily: (*GobbableMetricFamily)(mf),
		}
	}
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
	return d.Decode(&dms.metricGroups)
}

func copyMetricFamily(mf *dto.MetricFamily) *dto.MetricFamily {
	return &dto.MetricFamily{
		Name:   mf.Name,
		Help:   mf.Help,
		Type:   mf.Type,
		Metric: append([]*dto.Metric{}, mf.Metric...),
	}
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
