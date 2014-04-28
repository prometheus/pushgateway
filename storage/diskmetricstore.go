package storage

import (
	"sync"
	"time"

	dto "github.com/prometheus/client_model/go"
)

const (
	writeQueueCapacity = 1000
)

type jobToInstanceMap map[string]instanceToNameMap
type instanceToNameMap map[string]nameToTimestampedMetricFamilyMap
type nameToTimestampedMetricFamilyMap map[string]timestampedMetricFamily

type DiskMetricStore struct {
	lock           sync.RWMutex
	writeQueue     chan WriteRequest
	drain          chan struct{}
	done           chan error
	metricFamilies jobToInstanceMap
}

func NewDiskMetricStore(
	persistenceFile string,
	persistenceDuration time.Duration,
) (*DiskMetricStore, error) {
	dms := &DiskMetricStore{
		writeQueue:     make(chan WriteRequest, writeQueueCapacity),
		drain:          make(chan struct{}),
		done:           make(chan error),
		metricFamilies: jobToInstanceMap{},
	}
	// TODO read from file
	go dms.loop()
	return dms, nil
}

func (dms *DiskMetricStore) SubmitWriteRequest(req WriteRequest) {
	dms.writeQueue <- req
}

func (dms *DiskMetricStore) GetMetricFamilies() []*dto.MetricFamily {
	result := []*dto.MetricFamily{}
	dms.lock.RLock()
	defer dms.lock.RUnlock()
	for _, instances := range dms.metricFamilies {
		for _, names := range instances {
			for _, tmf := range names {
				result = append(result, tmf.metricFamily)
			}
		}
	}
	return result
}

func (dms *DiskMetricStore) Shutdown() error {
	close(dms.drain)
	return <-dms.done
}

func (dms *DiskMetricStore) loop() {
	for {
		select {
		case wr := <-dms.writeQueue:
			dms.processWriteRequest(wr)
			// TODO some timer for persistence
		case <-dms.drain:
			// Now draining...
			for {
				select {
				case wr := <-dms.writeQueue:
					dms.processWriteRequest(wr)
				default:
					// TODO persist to file
					dms.done <- nil
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
			instances = instanceToNameMap{}
			dms.metricFamilies[wr.Job] = instances
		}
		names, ok := instances[wr.Instance]
		if !ok {
			names = nameToTimestampedMetricFamilyMap{}
			instances[wr.Instance] = names
		}
		names[name] = timestampedMetricFamily{
			timestamp:    wr.Timestamp,
			metricFamily: mf,
		}
	}
}
