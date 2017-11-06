// Copyright 2017 The Prometheus Authors
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
	"io"
	"net/http"

	"github.com/monzo/pushgateway/storage"
)

func Healthy(
	ms storage.MetricStore,
) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		err := ms.Healthy()
		if err == nil {
			io.WriteString(w, "OK")
		} else {
			http.Error(w, err.Error(), 500)
		}
	}
}

func Ready(
	ms storage.MetricStore,
) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		err := ms.Ready()
		if err == nil {
			io.WriteString(w, "OK")
		} else {
			http.Error(w, err.Error(), 500)
		}
	}
}
