// Copyright 2019 The Prometheus Authors
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

package handler

import (
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"

	"github.com/prometheus/pushgateway/storage"
)

func WipeMetricStore(
	ms storage.MetricStore,
	logger log.Logger) http.Handler {

	// TODO: Should we return promhttp.InstrumentHandlerCounter and count calls to this endpoint?
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		level.Debug(logger).Log("msg", "start wiping metric store")
		if err := ms.Wipe(); err != nil {
			errorMsg := "wiping metric store"
			level.Debug(logger).Log("msg", errorMsg, "err", err)
			http.Error(w, errors.Wrap(err, errorMsg).Error(), http.StatusInternalServerError)
			w.Write([]byte("500 - " + err.Error()))
		}
		return
	})
}
