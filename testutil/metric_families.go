package testutil

import (
	"github.com/golang/protobuf/proto"
	dto "github.com/prometheus/client_model/go"
)

// MetricFamiliesMap creates the map needed in the MetricFamilies field of a
// WriteRequest from the provided reference metric families. While doing so, it
// creates deep copies of the metric families so that modifications that might
// happen during processing of the WriteRequest will not affect the reference
// metric families.
func MetricFamiliesMap(mfs ...*dto.MetricFamily) map[string]*dto.MetricFamily {
	m := map[string]*dto.MetricFamily{}
	for _, mf := range mfs {
		buf, err := proto.Marshal(mf)
		if err != nil {
			panic(err)
		}
		mfCopy := &dto.MetricFamily{}
		if err := proto.Unmarshal(buf, mfCopy); err != nil {
			panic(err)
		}
		m[mf.GetName()] = mfCopy
	}
	return m
}
