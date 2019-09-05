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
	"os"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	"github.com/prometheus/pushgateway/storage"
)

func WipePersistentFile(
	dms *storage.DiskMetricStore,
	newStorage func() *storage.DiskMetricStore,
	logger log.Logger) http.Handler {

	// TODO: Should we return promhttp.InstrumentHandlerCounter and count calls to this endpoint?
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		level.Debug(logger).Log("msg", "wiping persistence file")
		// Since we plan to wipe out the entire storage we actually don't care too much
		// if there was an error persisting the last metrics, still I can see some value in logging
		// potential errors for troubleshoting
		if err := dms.Shutdown(); err != nil {
			level.Error(logger).Log("msg", "problem shutting down metric storage", "err", err)
		}

		n := newStorage()
		// Delete persitence file
		// TODO: should we handle the error from os.Remove? What exactly should we be doing?
		os.Remove(n.GetPersistenceFile())

		// Set the value of the external disk metric store to the dereferenced value of a new disk metric storage
		// effectivly having a brand new storage running and ready to expose and persist pushed metrics
		*dms = *n
		return
	})
}
