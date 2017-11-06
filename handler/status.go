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
	"fmt"
	"html"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/monzo/pushgateway/storage"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
)

type data struct {
	MetricGroups storage.GroupingKeyToMetricGroup
	Flags        map[string]string
	BuildInfo    map[string]string
	Birth        time.Time
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
func Status(
	ms storage.MetricStore,
	assetFunc func(string) ([]byte, error),
	flags map[string]string,
) func(http.ResponseWriter, *http.Request) {
	birth := time.Now()
	return func(w http.ResponseWriter, _ *http.Request) {
		t := template.New("status")
		t.Funcs(template.FuncMap{
			"value": func(f float64) string {
				return strconv.FormatFloat(f, 'f', -1, 64)
			},
		})
		tpl, err := assetFunc("template.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Errorf("Error loading template.html, %v", err.Error())
			return
		}
		_, err = t.Parse(string(tpl))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Errorf("Error parsing template, %v", err.Error())
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
			Flags:        flags,
			BuildInfo:    buildInfo,
			Birth:        birth,
		}
		err = t.Execute(w, d)
		if err != nil {
			// Hack to get a visible error message right at the top.
			fmt.Fprintf(w, `<div id="template-error" class="alert alert-danger">Error executing template: %s</div>`, html.EscapeString(err.Error()))
			fmt.Fprintln(w, `<script>$("#template-error").prependTo("body")</script>`)
		}
	}
}
