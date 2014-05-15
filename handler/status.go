package handler

import (
	"fmt"
	"html"
	"html/template"
	"net/http"
	"time"

	"github.com/prometheus/pushgateway/storage"
)

type data struct {
	MetricFamilies storage.JobToInstanceMap
	Flags          map[string]string
	BuildInfo      map[string]string
	Birth          time.Time
	counter        int
}

func (d *data) Count() int {
	d.counter++
	return d.counter
}

func (_ data) FormatTimestamp(ts int64) string {
	return time.Unix(ts/1000, ts%1000*1000000).String()
}

func Status(
	ms storage.MetricStore,
	flags map[string]string,
	buildInfo map[string]string,
) func(http.ResponseWriter) {
	birth := time.Now()
	return func(w http.ResponseWriter) {
		t, err := template.ParseFiles("handler/template.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		d := &data{
			MetricFamilies: ms.GetMetricFamiliesMap(),
			Flags:          flags,
			BuildInfo:      buildInfo,
			Birth:          birth,
		}
		err = t.Execute(w, d)
		if err != nil {
			// Hack to get a visible error message right at the top.
			fmt.Fprintf(w, `<div id="template-error" class="alert alert-danger">Error executing template: %s</div>`, html.EscapeString(err.Error()))
			fmt.Fprintln(w, `<script>$( "#template-error" ).prependTo( "body" )</script>`)
		}
	}
}
