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
	"encoding/base64"
	"fmt"
	"html"
	"html/template"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	"github.com/prometheus/common/version"
	"github.com/prometheus/pushgateway/storage"
)

type data struct {
	MetricGroups storage.GroupingKeyToMetricGroup
	Flags        map[string]string
	BuildInfo    map[string]string
	Birth        time.Time
	PathPrefix   string
	counter      int
}

func (d *data) Count() int {
	d.counter++
	return d.counter
}

func (data) FormatTimestamp(ts int64) string {
	return time.Unix(ts/1000, ts%1000*1000000).String()
}

// Status serves the status page.
//
// The returned handler is already instrumented for Prometheus.
func Status(
	ms storage.MetricStore,
	root http.FileSystem,
	flags map[string]string,
	pathPrefix string,
	logger log.Logger,
) http.Handler {
	birth := time.Now()
	return InstrumentWithCounter(
		"status",
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t := template.New("status")
			t.Funcs(template.FuncMap{
				"value": func(f float64) string {
					return strconv.FormatFloat(f, 'f', -1, 64)
				},
				"timeFormat": func(t time.Time) string {
					return t.Format(time.RFC3339)
				},
				"base64": func(s string) string {
					return base64.RawURLEncoding.EncodeToString([]byte(s))
				},
			})

			f, err := root.Open("template.html")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				level.Error(logger).Log("msg", "error loading template.html", "err", err.Error())
				return
			}
			defer f.Close()
			tpl, err := ioutil.ReadAll(f)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				level.Error(logger).Log("msg", "error reading template.html", "err", err.Error())
				return
			}
			_, err = t.Parse(string(tpl))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				level.Error(logger).Log("msg", "error parsing template", "err", err.Error())
				return
			}

			buildInfo := map[string]string{
				"version":   version.Version,
				"revision":  version.Revision,
				"branch":    version.Branch,
				"buildUser": version.BuildUser,
				"buildDate": version.BuildDate,
				"goVersion": version.GoVersion,
			}

			d := &data{
				MetricGroups: ms.GetMetricFamiliesMap(),
				BuildInfo:    buildInfo,
				Birth:        birth,
				PathPrefix:   pathPrefix,
				Flags:        flags,
			}

			err = t.Execute(w, d)
			if err != nil {
				// Hack to get a visible error message right at the top.
				fmt.Fprintf(w, `<div id="template-error" class="alert alert-danger">Error executing template: %s</div>`, html.EscapeString(err.Error()))
				fmt.Fprintln(w, `<script>$("#template-error").prependTo("body")</script>`)
			}
		}),
	)
}
