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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/pushgateway/asset"
	"github.com/prometheus/pushgateway/storage"
)

func TestPathPrefixPresenceInPage(t *testing.T) {
	flags := map[string]string{
		"web.listen-address": ":9091",
		"web.telemetry-path": "/metrics",
		"web.external-url":   "http://web-external-url.com",
	}
	pathPrefix := "/foobar"

	ms := storage.NewDiskMetricStore("", time.Minute, nil, logger)
	status := Status(ms, asset.Assets, flags, pathPrefix, logger)
	defer ms.Shutdown()

	w := httptest.NewRecorder()
	status.ServeHTTP(w, &http.Request{})

	if http.StatusOK != w.Code {
		t.Fatalf("Wanted status %d, got %d", http.StatusOK, w.Code)
	}

	rawBody, err := ioutil.ReadAll(w.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	body := string(rawBody)

	if !strings.Contains(body, pathPrefix+"/static") {
		t.Errorf("Body does not contain %q.", pathPrefix+"/static")
		t.Log(body)
	}
}
