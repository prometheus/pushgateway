// Copyright 2020 The Prometheus Authors
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
package testutil

import (
	//nolint:staticcheck // Ignore SA1019. Dependencies use the deprecated package, so we have to, too.
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
