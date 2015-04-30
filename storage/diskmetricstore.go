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
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"

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
	metricFamilies  JobToInstanceMap
	persistenceFile string
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
) *DiskMetricStore {
	dms := &DiskMetricStore{
		writeQueue:      make(chan WriteRequest, writeQueueCapacity),
		drain:           make(chan struct{}),
		done:            make(chan error),
		metricFamilies:  JobToInstanceMap{},
		persistenceFile: persistenceFile,
	}
	if err := dms.restore(); err != nil {
		log.Print("Could not load persisted metrics: ", err)
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

	for _, instances := range dms.metricFamilies {
		for _, names := range instances {
			for name, tmf := range names {
				mf := tmf.MetricFamily
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
					if mf.GetHelp() != existingMF.GetHelp() || mf.GetType() != existingMF.GetType() {
						log.Printf(
							"Metric families '%s' and '%s' are inconsistent, help and type of the latter will have priority. This is bad. Fix your pushed metrics!",
							mf, existingMF,
						)
					}
					for _, metric := range mf.Metric {
						existingMF.Metric = append(existingMF.Metric, metric)
					}
				} else {
					mfStatByName[name] = mfStat{
						pos:    len(result),
						copied: false,
					}
					result = append(result, mf)
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

func (dms *DiskMetricStore) loop(persistenceInterval time.Duration) {
	lastPersist := time.Now()
	persistScheduled := false
	lastWrite := time.Time{}
	persistDone := make(chan time.Time)
	var persistTimer *time.Timer

	checkPersist := func() {
		if !persistScheduled && lastWrite.After(lastPersist) {
			persistTimer = time.AfterFunc(
				persistenceInterval-lastWrite.Sub(lastPersist),
				func() {
					persistStarted := time.Now()
					if err := dms.persist(); err != nil {
						log.Print("Error persisting metrics: ", err)
					} else {
						log.Printf(
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
	if wr.MetricFamilies == nil {
		// Delete.
		if wr.Instance == "" {
			delete(dms.metricFamilies, wr.Job)
		} else {
			instances, ok := dms.metricFamilies[wr.Job]
			if ok {
				delete(instances, wr.Instance)
				if len(instances) == 0 {
					// Clean up empty instance maps to not leak memory.
					delete(dms.metricFamilies, wr.Job)
				}
			}
		}
		return
	}
	// Update.
	for name, mf := range wr.MetricFamilies {
		instances, ok := dms.metricFamilies[wr.Job]
		if !ok {
			instances = InstanceToNameMap{}
			dms.metricFamilies[wr.Job] = instances
		}
		names, ok := instances[wr.Instance]
		if !ok {
			names = NameToTimestampedMetricFamilyMap{}
			instances[wr.Instance] = names
		}
		names[name] = TimestampedMetricFamily{
			Timestamp:    wr.Timestamp,
			MetricFamily: mf,
		}
	}
}

func (dms *DiskMetricStore) getTimestampedMetricFamilies() []TimestampedMetricFamily {
	result := []TimestampedMetricFamily{}
	dms.lock.RLock()
	defer dms.lock.RUnlock()
	for _, i2n := range dms.metricFamilies {
		for _, n2tmf := range i2n {
			for _, tmf := range n2tmf {
				result = append(result, tmf)
			}
		}
	}
	return result
}

// GetMetricFamiliesMap implements the MetricStore interface.
func (dms *DiskMetricStore) GetMetricFamiliesMap() JobToInstanceMap {
	dms.lock.RLock()
	defer dms.lock.RUnlock()
	j2iCopy := make(JobToInstanceMap, len(dms.metricFamilies))
	for j, i2n := range dms.metricFamilies {
		i2nCopy := make(InstanceToNameMap, len(i2n))
		j2iCopy[j] = i2nCopy
		for i, n2tmf := range i2n {
			n2tmfCopy := make(NameToTimestampedMetricFamilyMap, len(n2tmf))
			i2nCopy[i] = n2tmfCopy
			for n, tmf := range n2tmf {
				n2tmfCopy[n] = tmf
			}
		}
	}
	return j2iCopy
}

func (dms *DiskMetricStore) persist() error {
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
	for _, tmf := range dms.getTimestampedMetricFamilies() {
		if err := writeTimestampedMetricFamily(e, tmf); err != nil {
			f.Close()
			os.Remove(inProgressFileName)
			return err
		}
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
	if err != nil {
		return err
	}
	defer f.Close()
	var tmf TimestampedMetricFamily
	for d := gob.NewDecoder(f); err == nil; tmf, err = readTimestampedMetricFamily(d) {
		if len(tmf.MetricFamily.GetMetric()) == 0 {
			continue // No metric in this MetricFamily.
		}
		name := tmf.MetricFamily.GetName()
		var job, instance string
		for _, lp := range tmf.MetricFamily.GetMetric()[0].GetLabel() {
			// With the way the pushgateway persists things, all
			// metrics in a single MetricFamily proto message share
			// the same job and instance label. So we only have to
			// peek at the first metric to find it.
			switch lp.GetName() {
			case "job":
				job = lp.GetValue()
			case "instance":
				instance = lp.GetValue()
			}
			if job != "" && instance != "" {
				break
			}
		}
		instances, ok := dms.metricFamilies[job]
		if !ok {
			instances = InstanceToNameMap{}
			dms.metricFamilies[job] = instances
		}
		names, ok := instances[instance]
		if !ok {
			names = NameToTimestampedMetricFamilyMap{}
			instances[instance] = names
		}
		names[name] = tmf
	}
	if err == io.EOF {
		return nil
	}
	return err
}

func writeTimestampedMetricFamily(e *gob.Encoder, tmf TimestampedMetricFamily) error {
	// Since we have to serialize the timestamp, too, we are using gob for
	// everything (and not pbutil.WriteDelimited).
	buffer, err := proto.Marshal(tmf.MetricFamily)
	if err != nil {
		return err
	}
	if err := e.Encode(buffer); err != nil {
		return err
	}
	if err := e.Encode(tmf.Timestamp); err != nil {
		return err
	}
	return nil
}

func readTimestampedMetricFamily(d *gob.Decoder) (TimestampedMetricFamily, error) {
	var buffer []byte
	if err := d.Decode(&buffer); err != nil {
		return TimestampedMetricFamily{}, err
	}
	mf := &dto.MetricFamily{}
	if err := proto.Unmarshal(buffer, mf); err != nil {
		return TimestampedMetricFamily{}, err
	}
	var timestamp time.Time
	if err := d.Decode(&timestamp); err != nil {
		return TimestampedMetricFamily{}, err
	}
	return TimestampedMetricFamily{MetricFamily: mf, Timestamp: timestamp}, nil
}

func copyMetricFamily(mf *dto.MetricFamily) *dto.MetricFamily {
	return &dto.MetricFamily{
		Name:   mf.Name,
		Help:   mf.Help,
		Type:   mf.Type,
		Metric: append([]*dto.Metric{}, mf.Metric...),
	}
}
