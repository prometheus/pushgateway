package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type Label struct {
	Key, Val string
}

type Sample struct {
	Name      string
	Value     float64
	Timestamp int64
	Labels    []Label
}

type Metrics struct {
	Samples []Sample
	Expires time.Time
}

type instance struct {
	Job, Name string
}

type cache struct {
	m map[instance]Metrics
	sync.RWMutex
}

func newCache() *cache {
	return &cache{m: map[instance]Metrics{}}
}

func (c *cache) Get(jobName, instanceName string) (Metrics, bool) {
	c.RLock()
	defer c.RUnlock()

	metrics, ok := c.m[instance{jobName, instanceName}]
	return metrics, ok
}

func (c *cache) Set(jobName, instanceName string, m Metrics) {
	c.Lock()
	defer c.Unlock()

	c.m[instance{jobName, instanceName}] = m
}

// Samples implements SampleProducer. f is called repeatedly with Samples in
// the cache.
func (c *cache) Samples(f func(Sample) error) error {
	c.RLock()
	defer c.RUnlock()

	var labels = make([]Label, 0, 2)

	for instance, metrics := range c.m {
		labels = append(
			labels[:0],
			Label{Key: "job", Val: instance.Job},
			Label{Key: "instance", Val: instance.Name},
		)

		for _, sample := range metrics.Samples {
			sample.Labels = append(labels[:2], sample.Labels...)

			if err := f(sample); err != nil {
				return err
			}
		}
	}

	return nil
}

// String implements expvar.Var, returning a JSON-encoded string of the cache's
// contents.
func (c *cache) String() string {
	c.RLock()
	defer c.RUnlock()

	var b bytes.Buffer
	fmt.Fprintf(&b, "{\n")
	first := true

	for instance, metrics := range c.m {
		v, _ := json.Marshal(metrics)

		if !first {
			fmt.Fprintf(&b, ", ")
		}
		fmt.Fprintf(&b, "\"%s | %s\": %s", instance.Job, instance.Name, v)
		first = false
	}

	fmt.Fprintf(&b, "}")

	return string(b.String())
}

// Evict removes any expired Metrics on each tick.
func (c *cache) Evict(tick <-chan time.Time) {
	for now := range tick {
		c.Lock()

		for instance, metrics := range c.m {
			if now.After(metrics.Expires) {
				delete(c.m, instance)
			}
		}

		c.Unlock()
	}
}
