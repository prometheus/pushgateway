package handler

import (
	"bytes"

	"github.com/prometheus/client_golang/text"

	"github.com/prometheus/pushgateway/storage"
)

func Status(ms storage.MetricStore) func() []byte {
	return func() []byte {
		var buf bytes.Buffer
		for _, mf := range ms.GetMetricFamilies() {
			text.MetricFamilyToText(&buf, mf)
		}
		return buf.Bytes()
	}
}
