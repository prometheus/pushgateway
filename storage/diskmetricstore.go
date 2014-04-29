package storage

import (
	"encoding/gob"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"code.google.com/p/goprotobuf/proto"
	dto "github.com/prometheus/client_model/go"
)

const (
	writeQueueCapacity = 1000
)

type jobToInstanceMap map[string]instanceToNameMap
type instanceToNameMap map[string]nameToTimestampedMetricFamilyMap
type nameToTimestampedMetricFamilyMap map[string]timestampedMetricFamily

type DiskMetricStore struct {
	lock            sync.RWMutex
	writeQueue      chan WriteRequest
	drain           chan struct{}
	done            chan error
	metricFamilies  jobToInstanceMap
	persistenceFile string
}

func NewDiskMetricStore(
	persistenceFile string,
	persistenceDuration time.Duration,
) *DiskMetricStore {
	dms := &DiskMetricStore{
		writeQueue:      make(chan WriteRequest, writeQueueCapacity),
		drain:           make(chan struct{}),
		done:            make(chan error),
		metricFamilies:  jobToInstanceMap{},
		persistenceFile: persistenceFile,
	}
	if err := dms.restore(); err != nil {
		log.Print("Could not load persisted metrics: ", err)
	}
	go dms.loop(persistenceDuration)
	return dms
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

func (dms *DiskMetricStore) loop(persistenceDuration time.Duration) {
	lastPersist := time.Now() // If this IsZero(), persisting is scheduled.
	lastWrite := time.Time{}
	persistDone := make(chan time.Time)

	checkPersist := func() {
		if !lastPersist.IsZero() && lastWrite.After(lastPersist) {
			time.AfterFunc(
				persistenceDuration-lastWrite.Sub(lastPersist),
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
			lastPersist = time.Time{} // Mark persisting as scheduled.
		}
	}

	for {
		select {
		case wr := <-dms.writeQueue:
			dms.processWriteRequest(wr)
			lastWrite = time.Now()
			checkPersist()
		case lastPersist = <-persistDone:
			checkPersist() // In case something has been written in the meantime.
		case <-dms.drain:
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

func (dms *DiskMetricStore) getTimestampedMetricFamilies() []timestampedMetricFamily {
	result := []timestampedMetricFamily{}
	dms.lock.RLock()
	defer dms.lock.RUnlock()
	for _, instances := range dms.metricFamilies {
		for _, names := range instances {
			for _, tmf := range names {
				result = append(result, tmf)
			}
		}
	}
	return result
}

func (dms *DiskMetricStore) persist() error {
	if dms.persistenceFile == "" {
		return nil
	}
	inProgressFileName := dms.persistenceFile + ".in_progress"
	f, err := os.Create(inProgressFileName)
	if err != nil {
		return err
	}
	e := gob.NewEncoder(f)
	for _, tmf := range dms.getTimestampedMetricFamilies() {
		if err := writeTimestampedMetricFamily(e, tmf); err != nil {
			f.Close()
			os.Remove(inProgressFileName)
			return err
		}
	}
	if err := f.Close(); err != nil {
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
	var tmf timestampedMetricFamily
	for d := gob.NewDecoder(f); err == nil; tmf, err = readTimestampedMetricFamily(d) {
		if len(tmf.metricFamily.GetMetric()) == 0 {
			continue // No metric in this MetricFamily.
		}
		name := tmf.metricFamily.GetName()
		var job, instance string
		for _, lp := range tmf.metricFamily.GetMetric()[0].GetLabel() {
			// With the way the pushgateway persists thing, all
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
			instances = instanceToNameMap{}
			dms.metricFamilies[job] = instances
		}
		names, ok := instances[instance]
		if !ok {
			names = nameToTimestampedMetricFamilyMap{}
			instances[instance] = names
		}
		names[name] = tmf
	}
	if err == io.EOF {
		return nil
	}
	return err
}

func writeTimestampedMetricFamily(e *gob.Encoder, tmf timestampedMetricFamily) error {
	// Since we have to serialize the timestamp, too, we are using gob for
	// everything (and not ext.WriteDelimited).
	buffer, err := proto.Marshal(tmf.metricFamily)
	if err != nil {
		return err
	}
	if err := e.Encode(buffer); err != nil {
		return err
	}
	if err := e.Encode(tmf.timestamp); err != nil {
		return err
	}
	return nil
}

func readTimestampedMetricFamily(d *gob.Decoder) (timestampedMetricFamily, error) {
	var buffer []byte
	if err := d.Decode(&buffer); err != nil {
		return timestampedMetricFamily{}, err
	}
	mf := &dto.MetricFamily{}
	if err := proto.Unmarshal(buffer, mf); err != nil {
		return timestampedMetricFamily{}, err
	}
	var timestamp time.Time
	if err := d.Decode(&timestamp); err != nil {
		return timestampedMetricFamily{}, err
	}
	return timestampedMetricFamily{metricFamily: mf, timestamp: timestamp}, nil
}
