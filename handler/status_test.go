// Copyright 2014 The Prometheus Authors
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
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/pushgateway/asset"
	"github.com/prometheus/pushgateway/storage"
)

func TestExternalURLPresenceInPage(t *testing.T) {
	flags := map[string]string{
		"web.listen-address": ":9091",
		"web.telemetry-path": "/metrics",
		"web.external-url":   "http://web-external-url.com",
	}

	ms := storage.NewDiskMetricStore("", time.Minute, nil)
	status := Status(ms, asset.Assets, flags)
	defer ms.Shutdown()

	w := httptest.NewRecorder()
	status.ServeHTTP(w, &http.Request{})

	if http.StatusOK != w.Code {
		t.Fatalf("Wanted status %d, got %d", http.StatusOK, w.Code)
	}

	var rawBody []byte
	w.Result().Body.Read(rawBody)
	body := string(rawBody)

	if index := strings.Index(body, flags["web.external-url"]); index > 0 {
		t.Errorf("Wanted index of %q > 0 , got %d", flags["web.external-url"], index)
	}
}
