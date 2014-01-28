package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"code.google.com/p/goprotobuf/proto"
	dto "github.com/prometheus/client_model/go"
)

func TestPushHandler(t *testing.T) {
	var (
		cache   = newCache()
		handler = pushHandler(cache)

		buf = new(bytes.Buffer)
		enc = dto.NewEncoder(buf)
	)

	enc.Encode(&dto.Sample{
		Name:  proto.String("request_count"),
		Value: proto.Float64(-42),
		Label: []*dto.Label{
			{Key: proto.String("label_name"), Val: proto.String("label_value")},
		},
	})

	enc.Encode(&dto.Sample{
		Name:  proto.String("request_count"),
		Value: proto.Float64(6.4),
		Time:  proto.Int64(1390926463),
		Label: []*dto.Label{
			{Key: proto.String("another_label_name"), Val: proto.String("another_label_value")},
		},
	})

	uri := url.URL{
		Path: "/",
		RawQuery: url.Values{
			":job":      []string{"exporter"},
			":instance": []string{"app001"},
		}.Encode(),
	}

	req, _ := http.NewRequest("PUT", uri.String(), buf)
	req.Header.Set("Content-Type", sampleContentType)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected %q, got %q", http.StatusText(http.StatusNoContent), http.StatusText(w.Code))
	}

	metrics, ok := cache.Get("exporter", "app001")
	if !ok {
		t.Fatal("expected metrics to be in cache")
	}

	if len(metrics.Samples) != 2 {
		t.Fatalf("expected sample length to be 2, was %d", len(metrics.Samples))
	}

	var sample = metrics.Samples[0]

	if sample.Name != "request_count" {
		t.Fatal("incorrect sample")
	}

	if sample.Value != -42 {
		t.Fatal("incorrect sample")
	}

	if sample.Timestamp == 0 {
		t.Fatal("incorrect sample")
	}

	if !reflect.DeepEqual(sample.Labels, []Label{{"label_name", "label_value"}}) {
		t.Fatal("incorrect sample")
	}

	sample = metrics.Samples[1]

	if sample.Name != "request_count" {
		t.Fatal("incorrect sample")
	}

	if sample.Value != 6.4 {
		t.Fatal("incorrect sample")
	}

	if sample.Timestamp != 1390926463 {
		t.Fatal("incorrect sample")
	}

	if !reflect.DeepEqual(sample.Labels, []Label{{"another_label_name", "another_label_value"}}) {
		t.Fatal("incorrect sample")
	}
}
