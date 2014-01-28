package main

import (
	"log"
	"net/http"
	"sync"

	dto "github.com/prometheus/client_model/go"
)

type SampleProducer interface {
	Samples(func(Sample) error) error
}

// The registry allows SampleProducers to publish their Samples to be written
// in negotiated formats over HTTP.
type registry struct {
	producers []SampleProducer

	http.Handler
	sync.RWMutex
}

func newRegistry() *registry {
	r := &registry{}
	r.Handler = negotiate([]negotiator{
		{"text/plain", http.HandlerFunc(r.textHandler)},
		{sampleContentType, http.HandlerFunc(r.protobufHandler)},
	})

	return r
}

func (reg *registry) Publish(s SampleProducer) {
	reg.Lock()
	defer reg.Unlock()

	reg.producers = append(reg.producers, s)
}

// textHandler writes a text encoding of the registry's published samples.
//
// TODO: this is not a standardized format, but exists so you see something if
// you hit the metrics handler and can't handle the binary protobuf encoding.
func (reg *registry) textHandler(w http.ResponseWriter, r *http.Request) {
	reg.RLock()
	defer reg.RUnlock()

	var enc = newEncoder(newTextEncoder(w))

	for _, s := range reg.producers {
		err := s.Samples(func(s Sample) error {
			return enc.Encode(&s)
		})

		if err != nil {
			log.Println("error writing samples to stream:", err)
			return
		}
	}
}

// protobufHandler writes a binary protobuf encoding of the registry's
// published samples.
func (reg *registry) protobufHandler(w http.ResponseWriter, r *http.Request) {
	reg.RLock()
	defer reg.RUnlock()

	var enc = newEncoder(dto.NewEncoder(w))

	for _, s := range reg.producers {
		err := s.Samples(func(s Sample) error {
			return enc.Encode(&s)
		})

		if err != nil {
			log.Println("error writing samples to stream:", err)
			return
		}
	}
}
