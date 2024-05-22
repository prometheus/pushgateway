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
package histogram

import (
	"fmt"

	dto "github.com/prometheus/client_model/go"
	model "github.com/prometheus/prometheus/model/histogram"
)

type APIBucket[BC model.BucketCount] struct {
	Boundaries   uint64
	Lower, Upper float64
	Count        BC
}

func NewModelHistogram(ch *dto.Histogram) (*model.Histogram, *model.FloatHistogram) {
	if ch.GetSampleCountFloat() > 0 || ch.GetZeroCountFloat() > 0 {
		// It is a float histogram.
		fh := model.FloatHistogram{
			Count:           ch.GetSampleCountFloat(),
			Sum:             ch.GetSampleSum(),
			ZeroThreshold:   ch.GetZeroThreshold(),
			ZeroCount:       ch.GetZeroCountFloat(),
			Schema:          ch.GetSchema(),
			PositiveSpans:   make([]model.Span, len(ch.GetPositiveSpan())),
			PositiveBuckets: ch.GetPositiveCount(),
			NegativeSpans:   make([]model.Span, len(ch.GetNegativeSpan())),
			NegativeBuckets: ch.GetNegativeCount(),
		}
		for i, span := range ch.GetPositiveSpan() {
			fh.PositiveSpans[i].Offset = span.GetOffset()
			fh.PositiveSpans[i].Length = span.GetLength()
		}
		for i, span := range ch.GetNegativeSpan() {
			fh.NegativeSpans[i].Offset = span.GetOffset()
			fh.NegativeSpans[i].Length = span.GetLength()
		}
		return nil, &fh
	}
	h := model.Histogram{
		Count:           ch.GetSampleCount(),
		Sum:             ch.GetSampleSum(),
		ZeroThreshold:   ch.GetZeroThreshold(),
		ZeroCount:       ch.GetZeroCount(),
		Schema:          ch.GetSchema(),
		PositiveSpans:   make([]model.Span, len(ch.GetPositiveSpan())),
		PositiveBuckets: ch.GetPositiveDelta(),
		NegativeSpans:   make([]model.Span, len(ch.GetNegativeSpan())),
		NegativeBuckets: ch.GetNegativeDelta(),
	}
	for i, span := range ch.GetPositiveSpan() {
		h.PositiveSpans[i].Offset = span.GetOffset()
		h.PositiveSpans[i].Length = span.GetLength()
	}
	for i, span := range ch.GetNegativeSpan() {
		h.NegativeSpans[i].Offset = span.GetOffset()
		h.NegativeSpans[i].Length = span.GetLength()
	}
	return &h, nil
}

func BucketsAsJson[BC model.BucketCount](buckets []APIBucket[BC]) [][]interface{} {
	ret := make([][]interface{}, len(buckets))
	for i, b := range buckets {
		ret[i] = []interface{}{b.Boundaries, fmt.Sprintf("%v", b.Lower), fmt.Sprintf("%v", b.Upper), fmt.Sprintf("%v", b.Count)}
	}
	return ret
}

func GetAPIBuckets(h *model.Histogram) []APIBucket[uint64] {
	var apiBuckets []APIBucket[uint64]
	var nBuckets []model.Bucket[uint64]
	for it := h.NegativeBucketIterator(); it.Next(); {
		bucket := it.At()
		if bucket.Count != 0 {
			nBuckets = append(nBuckets, it.At())
		}
	}
	for i := len(nBuckets) - 1; i >= 0; i-- {
		apiBuckets = append(apiBuckets, makeBucket[uint64](nBuckets[i]))
	}

	if h.ZeroCount != 0 {
		apiBuckets = append(apiBuckets, makeBucket[uint64](h.ZeroBucket()))
	}

	for it := h.PositiveBucketIterator(); it.Next(); {
		bucket := it.At()
		if bucket.Count != 0 {
			apiBuckets = append(apiBuckets, makeBucket[uint64](bucket))
		}
	}
	return apiBuckets
}

func GetAPIFloatBuckets(h *model.FloatHistogram) []APIBucket[float64] {
	var apiBuckets []APIBucket[float64]
	var nBuckets []model.Bucket[float64]
	for it := h.NegativeBucketIterator(); it.Next(); {
		bucket := it.At()
		if bucket.Count != 0 {
			nBuckets = append(nBuckets, it.At())
		}
	}
	for i := len(nBuckets) - 1; i >= 0; i-- {
		apiBuckets = append(apiBuckets, makeBucket[float64](nBuckets[i]))
	}

	if h.ZeroCount != 0 {
		apiBuckets = append(apiBuckets, makeBucket[float64](h.ZeroBucket()))
	}

	for it := h.PositiveBucketIterator(); it.Next(); {
		bucket := it.At()
		if bucket.Count != 0 {
			apiBuckets = append(apiBuckets, makeBucket[float64](bucket))
		}
	}
	return apiBuckets
}

func makeBucket[BC model.BucketCount](bucket model.Bucket[BC]) APIBucket[BC] {
	boundaries := uint64(2) // () Exclusive on both sides AKA open interval.
	if bucket.LowerInclusive {
		if bucket.UpperInclusive {
			boundaries = 3 // [] Inclusive on both sides AKA closed interval.
		} else {
			boundaries = 1 // [) Inclusive only on lower end AKA right open.
		}
	} else {
		if bucket.UpperInclusive {
			boundaries = 0 // (] Inclusive only on upper end AKA left open.
		}
	}
	return APIBucket[BC]{
		Boundaries: boundaries,
		Lower:      bucket.Lower,
		Upper:      bucket.Upper,
		Count:      bucket.Count,
	}
}
