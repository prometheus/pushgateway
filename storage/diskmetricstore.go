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

	"github.com/prometheus/common/log"
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
	metricsTTL      time.Duration
	stripTimestamps bool
}

type mfStat struct {
	pos    int  // Where in the result slice is the MetricFamily?
	copied bool // Has the MetricFamily already been copied?
}

// NewDiskMetricStore returns a DiskMetricStore ready to use. To cleanly shut it
// down and free resources, the Shutdown() method has to be called.  If
// persistenceFile is the empty string, no persisting to disk will
// happen. Otherwise, a file of that name is used for persisting metrics to
// disk. If the file already exists, metrics are read from it as part of the
// start-up. Persisting is happening upon shutdown and after every write action,
// but the latter will only happen persistenceDuration after the previous
// persisting.
func NewDiskMetricStore(
	persistenceFile string,
	persistenceInterval time.Duration,
	metricsTTL time.Duration,
	stripTimestamps bool,
) *DiskMetricStore {
	// TODO: Do that outside of the constructor to allow the HTTP server to
	//  serve /-/healthy and /-/ready earlier.
	dms := &DiskMetricStore{
		writeQueue:      make(chan WriteRequest, writeQueueCapacity),
		drain:           make(chan struct{}),
		done:            make(chan error),
		metricGroups:    GroupingKeyToMetricGroup{},
		persistenceFile: persistenceFile,
		metricsTTL:      metricsTTL,
		stripTimestamps: stripTimestamps,
	}
	if err := dms.restore(); err != nil {
		log.Errorln("Could not load persisted metrics:", err)
	}

	go dms.loop(persistenceInterval)
	return dms
}

// SubmitWriteRequest implements the MetricStore interface.
func (dms *DiskMetricStore) SubmitWriteRequest(req WriteRequest) {
	dms.writeQueue <- req
}

// Iterates over all metrics in a metricFamily and returns a new metricFamily with any metrics that are too old removed
// returns a nil if there were no metrics left.
func filterOutOldMetrics(mf *dto.MetricFamily, ttl time.Duration, stripTimestamps bool) *dto.MetricFamily {

	// if the ttl is 0 then we return the original metricfamily
	if ttl == 0 {
		return mf
	}

	// We won't include any metrics with timestamp older than this.
	threshold := (time.Now().UnixNano() / int64(time.Millisecond)) - int64(ttl.Seconds()*1000)
	filteredMetrics := []*dto.Metric{}
	for _, metric := range mf.Metric {
		if metric.TimestampMs != nil && *metric.TimestampMs > threshold {
			if stripTimestamps {
				filteredMetrics = append(filteredMetrics, &dto.Metric{
					Label:            metric.Label,
					Gauge:            metric.Gauge,
					Counter:          metric.Counter,
					Summary:          metric.Summary,
					Untyped:          metric.Untyped,
					Histogram:        metric.Histogram,
					XXX_unrecognized: mf.XXX_unrecognized,})
			} else {
				filteredMetrics = append(filteredMetrics, metric)
			}
		}
	}
	// if there were no metrics it makes no sense to make useless structs
	if len(filteredMetrics) == 0 {
		return nil
	}
	// We do have sensible metric(s) make a metricFamily to contain them.
	filteredMetricFamily := &dto.MetricFamily{
		Name:             mf.Name,
		Help:             mf.Help,
		Type:             mf.Type,
		Metric:           filteredMetrics,
		XXX_unrecognized: mf.XXX_unrecognized,
	}
	return filteredMetricFamily
}

// GetMetricFamilies implements the MetricStore interface.
func (dms *DiskMetricStore) GetMetricFamilies() []*dto.MetricFamily {
	result := []*dto.MetricFamily{}
	mfStatByName := map[string]mfStat{}

	dms.lock.RLock()
	defer dms.lock.RUnlock()

	for _, group := range dms.metricGroups {
		for name, tmf := range group.Metrics {
			mf := filterOutOldMetrics(tmf.GetMetricFamily(), dms.metricsTTL, dms.stripTimestamps)
			// if this metric family contained no valid metrics (they expired) exit this loop iteration early
			if mf == nil {
				continue
			}
			// We have metrics
			stat, exists := mfStatByName[name]
			if !exists {
				mfStatByName[name] = mfStat{
					pos:    len(result),
					copied: false,
				}
				result = append(result, mf)
			} else {
				existingMF := result[stat.pos]
				if !stat.copied {
					mfStatByName[name] = mfStat{
						pos:    stat.pos,
						copied: true,
					}
					existingMF = copyMetricFamily(existingMF)
					result[stat.pos] = existingMF
				}
				if mf.GetHelp() != existingMF.GetHelp() || mf.GetType() != existingMF.GetType() {
					log.Infof(
						"Metric families '%s' and '%s' are inconsistent, help and type of the latter will have priority. This is bad. Fix your pushed metrics!",
						mf, existingMF,
					)
				}
				for _, metric := range mf.Metric {
					existingMF.Metric = append(existingMF.Metric, metric)
				}
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
						log.Errorln("Error persisting metrics:", err)
					} else {
						log.Infof(
							"Metrics persisted to '%s'.",
							dms.persistenceFile,
						)
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
