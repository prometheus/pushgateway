package main

import (
	"runtime"
	"time"
)

type publisherFunc func(func(Sample) error) error

func (f publisherFunc) Samples(fn func(Sample) error) error {
	return f(fn)
}

var runtimeMetrics = publisherFunc(func(fn func(Sample) error) error {
	var (
		ms  runtime.MemStats
		now = time.Now().Unix()
	)
	runtime.ReadMemStats(&ms)

	samples := []Sample{
		{"instance_goroutine_count", float64(runtime.NumGoroutine()), now, nil},
		{"instance_allocated_bytes", float64(ms.Alloc), now, nil},
		{"instance_total_allocated_bytes", float64(ms.TotalAlloc), now, nil},
		{"instance_heap_allocated_bytes", float64(ms.HeapAlloc), now, nil},
		{"instance_gc_high_watermark_bytes", float64(ms.NextGC), now, nil},
		{"instance_gc_total_pause_ns", float64(ms.PauseTotalNs), now, nil},
		{"instance_gc_count", float64(ms.NumGC), now, nil},
	}

	for _, sample := range samples {
		if err := fn(sample); err != nil {
			return err
		}
	}

	return nil
})
