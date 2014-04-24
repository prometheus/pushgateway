package main

import (
	"reflect"
	"testing"
	"time"
)

func TestCacheSamples(t *testing.T) {
	var (
		samples = []Sample{
			{Name: "my_counter", Value: 1.2, Labels: []Label{{"a", "b"}}},
			{Name: "my_counter", Value: 2.2, Labels: []Label{{"b", "c"}, {"d", "e"}}},
			{Name: "my_counter", Value: 4.2, Labels: []Label{{"y", "z"}}},
		}

		cache = newCache()

		got []Sample

		expected = []Sample{
			{Name: "my_counter", Value: 1.2, Labels: []Label{{"job", "foo"}, {"instance", "bar"}, {"a", "b"}}},
			{Name: "my_counter", Value: 2.2, Labels: []Label{{"job", "foo"}, {"instance", "bar"}, {"b", "c"}, {"d", "e"}}},
			{Name: "my_counter", Value: 4.2, Labels: []Label{{"job", "foo"}, {"instance", "bar"}, {"y", "z"}}},
			{Name: "my_counter", Value: 1.2, Labels: []Label{{"job", "bar"}, {"instance", "baz"}, {"a", "b"}}},
			{Name: "my_counter", Value: 2.2, Labels: []Label{{"job", "bar"}, {"instance", "baz"}, {"b", "c"}, {"d", "e"}}},
			{Name: "my_counter", Value: 4.2, Labels: []Label{{"job", "bar"}, {"instance", "baz"}, {"y", "z"}}},
		}
	)

	cache.Set("foo", "bar", Metrics{Samples: samples})
	cache.Set("bar", "baz", Metrics{Samples: samples})

	cache.Samples(func(s Sample) error {
		got = append(got, s)
		return nil
	})

	if len(got) != len(expected) {
		t.Fatalf("expected %d samples, got %d", len(expected), len(got))
	}

outer:
	for _, expected := range expected {
		for _, got := range got {
			if reflect.DeepEqual(expected, got) {
				continue outer
			}
		}

		t.Fatalf("expected %v to be in output", expected)
	}
}

func TestCacheEvict(t *testing.T) {
	var (
		cache   = newCache()
		metrics = Metrics{Expires: time.Now()}
	)

	cache.Set("foo", "bar", metrics)

	tick := make(chan time.Time, 1)
	tick <- metrics.Expires.Add(-time.Second)
	close(tick)

	cache.Evict(tick)

	if _, ok := cache.Get("foo", "bar"); !ok {
		t.Fatal("did not expect metrics to be evicted")
	}

	tick = make(chan time.Time, 1)
	tick <- metrics.Expires.Add(time.Second)
	close(tick)

	cache.Evict(tick)

	if _, ok := cache.Get("foo", "bar"); ok {
		t.Fatal("expected metrics to be evicted")
	}
}
