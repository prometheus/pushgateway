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

package main

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/alecthomas/kingpin/v2"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/route"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"

	versioncollector "github.com/prometheus/client_golang/prometheus/collectors/version"
	dto "github.com/prometheus/client_model/go"
	promslogflag "github.com/prometheus/common/promslog/flag"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"

	"github.com/prometheus/pushgateway/asset"
	"github.com/prometheus/pushgateway/handler"
	"github.com/prometheus/pushgateway/storage"

	api_v1 "github.com/prometheus/pushgateway/api/v1"
)

func init() {
	prometheus.MustRegister(versioncollector.NewCollector("pushgateway"))
}

func main() {
	var (
		app                 = kingpin.New(filepath.Base(os.Args[0]), "The Pushgateway").UsageWriter(os.Stdout)
		webConfig           = webflag.AddFlags(app, ":9091")
		metricsPath         = app.Flag("web.telemetry-path", "Path under which to expose metrics.").Default("/metrics").String()
		externalURL         = app.Flag("web.external-url", "The URL under which the Pushgateway is externally reachable.").Default("").URL()
		routePrefix         = app.Flag("web.route-prefix", "Prefix for the internal routes of web endpoints. Defaults to the path of --web.external-url.").Default("").String()
		enableLifeCycle     = app.Flag("web.enable-lifecycle", "Enable shutdown via HTTP request.").Default("false").Bool()
		enableAdminAPI      = app.Flag("web.enable-admin-api", "Enable API endpoints for admin control actions.").Default("false").Bool()
		persistenceFile     = app.Flag("persistence.file", "File to persist metrics. If empty, metrics are only kept in memory.").Default("").String()
		persistenceInterval = app.Flag("persistence.interval", "The minimum interval at which to write out the persistence file.").Default("5m").Duration()
		pushUnchecked       = app.Flag("push.disable-consistency-check", "Do not check consistency of pushed metrics. DANGEROUS.").Default("false").Bool()
		pushUTF8Names       = app.Flag("push.enable-utf8-names", "Allow UTF-8 characters in metric and label names.").Default("false").Bool()
		promlogConfig       = promslog.Config{Style: promslog.GoKitStyle}
	)
	promslogflag.AddFlags(app, &promlogConfig)
	app.Version(version.Print("pushgateway"))
	app.HelpFlag.Short('h')
	kingpin.MustParse(app.Parse(os.Args[1:]))
	logger := promslog.New(&promlogConfig)

	*routePrefix = computeRoutePrefix(*routePrefix, *externalURL)
	externalPathPrefix := computeRoutePrefix("", *externalURL)

	logger.Info("starting pushgateway", "version", version.Info())
	logger.Info("Build context", "build_context", version.BuildContext())
	logger.Debug("external URL", "url", *externalURL)
	logger.Debug("path prefix used externally", "path", externalPathPrefix)
	logger.Debug("path prefix for internal routing", "path", *routePrefix)

	// flags is used to show command line flags on the status page.
	// Kingpin default flags are excluded as they would be confusing.
	flags := map[string]string{}
	boilerplateFlags := kingpin.New("", "").Version("")
	for _, f := range app.Model().Flags {
		if boilerplateFlags.GetFlag(f.Name) == nil {
			flags[f.Name] = f.Value.String()
		}
	}

	ms := storage.NewDiskMetricStore(*persistenceFile, *persistenceInterval, prometheus.DefaultGatherer, logger)

	if *pushUTF8Names {
		handler.EscapingScheme = model.ValueEncodingEscaping
		handler.ValidationScheme = model.UTF8Validation
	} else {
		handler.EscapingScheme = model.NoEscaping
		handler.ValidationScheme = model.LegacyValidation
	}

	// Create a Gatherer combining the DefaultGatherer and the metrics from the metric store.
	g := prometheus.Gatherers{
		prometheus.DefaultGatherer,
		prometheus.GathererFunc(func() ([]*dto.MetricFamily, error) { return ms.GetMetricFamilies(), nil }),
	}

	r := route.New()
	r.Get(*routePrefix+"/-/healthy", handler.Healthy(ms).ServeHTTP)
	r.Get(*routePrefix+"/-/ready", handler.Ready(ms).ServeHTTP)
	r.Get(
		path.Join(*routePrefix, *metricsPath),
		promhttp.HandlerFor(g, promhttp.HandlerOpts{
			ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError),
		}).ServeHTTP,
	)

	// Handlers for pushing and deleting metrics.
	pushAPIPath := *routePrefix + "/metrics"
	for _, suffix := range []string{"", handler.Base64Suffix} {
		jobBase64Encoded := suffix == handler.Base64Suffix
		r.Put(pushAPIPath+"/job"+suffix+"/:job/*labels", handler.Push(ms, true, !*pushUnchecked, jobBase64Encoded, logger))
		r.Post(pushAPIPath+"/job"+suffix+"/:job/*labels", handler.Push(ms, false, !*pushUnchecked, jobBase64Encoded, logger))
		r.Del(pushAPIPath+"/job"+suffix+"/:job/*labels", handler.Delete(ms, jobBase64Encoded, logger))
		r.Put(pushAPIPath+"/job"+suffix+"/:job", handler.Push(ms, true, !*pushUnchecked, jobBase64Encoded, logger))
		r.Post(pushAPIPath+"/job"+suffix+"/:job", handler.Push(ms, false, !*pushUnchecked, jobBase64Encoded, logger))
		r.Del(pushAPIPath+"/job"+suffix+"/:job", handler.Delete(ms, jobBase64Encoded, logger))
	}
	r.Get(*routePrefix+"/static/*filepath", handler.Static(asset.Assets, *routePrefix).ServeHTTP)

	statusHandler := handler.Status(ms, asset.Assets, flags, externalPathPrefix, logger)
	r.Get(*routePrefix+"/status", statusHandler.ServeHTTP)
	r.Get(*routePrefix+"/", statusHandler.ServeHTTP)

	// Re-enable pprof.
	r.Get(*routePrefix+"/debug/pprof/*pprof", handlePprof)

	quitCh := make(chan struct{})
	quitHandler := func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Requesting termination... Goodbye!")
		close(quitCh)
	}

	forbiddenAPINotEnabled := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Lifecycle API is not enabled."))
	}

	if *enableLifeCycle {
		r.Put(*routePrefix+"/-/quit", quitHandler)
		r.Post(*routePrefix+"/-/quit", quitHandler)
	} else {
		r.Put(*routePrefix+"/-/quit", forbiddenAPINotEnabled)
		r.Post(*routePrefix+"/-/quit", forbiddenAPINotEnabled)
	}

	r.Get("/-/quit", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("Only POST or PUT requests allowed."))
	})

	mux := http.NewServeMux()
	mux.Handle("/", decodeRequest(r))

	buildInfo := map[string]string{
		"version":   version.Version,
		"revision":  version.Revision,
		"branch":    version.Branch,
		"buildUser": version.BuildUser,
		"buildDate": version.BuildDate,
		"goVersion": version.GoVersion,
	}

	apiv1 := api_v1.New(logger, ms, flags, buildInfo)

	apiPath := "/api"
	if *routePrefix != "/" {
		apiPath = *routePrefix + apiPath
	}

	av1 := route.New()
	apiv1.Register(av1)
	if *enableAdminAPI {
		av1.Put("/admin/wipe", handler.WipeMetricStore(ms, logger).ServeHTTP)
	}

	mux.Handle(apiPath+"/v1/", http.StripPrefix(apiPath+"/v1", av1))

	server := &http.Server{Handler: mux}

	go shutdownServerOnQuit(server, quitCh, logger)
	err := web.ListenAndServe(server, webConfig, logger)

	// In the case of a graceful shutdown, do not log the error.
	if err == http.ErrServerClosed {
		logger.Info("HTTP server stopped")
	} else {
		logger.Error("HTTP server stopped", "err", err)
	}

	if err := ms.Shutdown(); err != nil {
		logger.Error("problem shutting down metric storage", "err", err)
	}
}

func decodeRequest(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close() // Make sure the underlying io.Reader is closed.
		switch contentEncoding := r.Header.Get("Content-Encoding"); strings.ToLower(contentEncoding) {
		case "gzip":
			gr, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			defer gr.Close()
			r.Body = gr
		case "snappy":
			r.Body = io.NopCloser(snappy.NewReader(r.Body))
		default:
			// Do nothing.
		}

		h.ServeHTTP(w, r)
	})
}

func handlePprof(w http.ResponseWriter, r *http.Request) {
	switch route.Param(r.Context(), "pprof") {
	case "/cmdline":
		pprof.Cmdline(w, r)
	case "/profile":
		pprof.Profile(w, r)
	case "/symbol":
		pprof.Symbol(w, r)
	default:
		pprof.Index(w, r)
	}
}

// computeRoutePrefix returns the effective route prefix based on the
// provided flag values for --web.route-prefix and
// --web.external-url. With prefix empty, the path of externalURL is
// used instead. A prefix "/" results in an empty returned prefix. Any
// non-empty prefix is normalized to start, but not to end, with "/".
func computeRoutePrefix(prefix string, externalURL *url.URL) string {
	if prefix == "" {
		prefix = externalURL.Path
	}

	if prefix == "/" {
		return ""
	}

	// Ensure prefix starts with "/".
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}

	prefix = strings.TrimSuffix(prefix, "/")

	return prefix
}

// shutdownServerOnQuit shutdowns the provided server upon closing the provided
// quitCh or upon receiving a SIGINT or SIGTERM.
func shutdownServerOnQuit(server *http.Server, quitCh <-chan struct{}, logger *slog.Logger) error {
	notifier := make(chan os.Signal, 1)
	signal.Notify(notifier, os.Interrupt, syscall.SIGTERM)

	select {
	case <-notifier:
		logger.Info("received SIGINT/SIGTERM; exiting gracefully...")
		break
	case <-quitCh:
		logger.Warn("received termination request via web service, exiting gracefully...")
		break
	}
	return server.Shutdown(context.Background())
}
