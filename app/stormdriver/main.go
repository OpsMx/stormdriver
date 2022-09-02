/*
 * Copyright 2022 OpsMx, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License")
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/OpsMx/go-app-base/birger"
	"github.com/OpsMx/go-app-base/httputil"
	"github.com/OpsMx/go-app-base/tracer"
	"github.com/OpsMx/go-app-base/util"
	"github.com/OpsMx/go-app-base/version"
	"github.com/skandragon/gohealthcheck/health"
)

const (
	appName = "stormdriver"
)

var (
	configFile = flag.String("configFile", "/app/config/stormdriver.yaml", "Configuration file location")

	// eg, http://localhost:14268/api/traces
	jaegerEndpoint = flag.String("jaeger-endpoint", "", "Jaeger collector endpoint")
	traceToStdout  = flag.Bool("traceToStdout", false, "log traces to stdout")
	traceRatio     = flag.Float64("traceRatio", 0.01, "ratio of traces to create, if incoming request is not traced")
	showversion    = flag.Bool("version", false, "show the version and exit")

	conf               *configuration
	healthchecker      = health.MakeHealth()
	tracerProvider     *tracer.TracerProvider
	clouddriverManager *ClouddriverManager
)

func main() {
	log.Printf("%s", version.VersionString())
	flag.Parse()
	if *showversion {
		os.Exit(0)
	}

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGTERM, syscall.SIGINT)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if *jaegerEndpoint != "" {
		*jaegerEndpoint = util.GetEnvar("JAEGER_TRACE_URL", "")
	}

	var err error
	tracerProvider, err = tracer.NewTracerProvider(*jaegerEndpoint, *traceToStdout, version.GitHash(), appName, *traceRatio)
	util.Check(err)
	defer tracerProvider.Shutdown(ctx)

	conf = loadConfigurationFile(*configFile)

	if len(conf.Clouddrivers) == 0 && conf.Controller.URL == "" {
		log.Printf("ERROR: no clouddrivers defined in config, and controller not configured.")
	}

	for _, cd := range conf.Clouddrivers {
		log.Printf("Clouddriver name: %s", cd.Name)
	}

	clouddriverManager = MakeClouddriverManager(conf.Clouddrivers, conf.SpinnakerUser)

	var controllerManager *birger.ControllerManager
	updateChan := make(chan birger.ServiceUpdate)
	if conf.Controller.URL != "" {
		controllerManager = birger.MakeControllerManager(conf.Controller, []string{"clouddriver"})

		caCert, err := controllerManager.GetCACertPEM()
		util.Check(err)
		cfg, err := makeTLSConfigWithCA(caCert)
		util.Check(err)
		httputil.SetTLSConfig(cfg)
		updateChan = controllerManager.UpdateChan

		healthchecker.AddCheck("controllerManager", false, controllerManager)
	}

	http.DefaultClient = httputil.NewHTTPClient(nil)

	go clouddriverManager.accountTracker(updateChan)

	for _, cd := range conf.Clouddrivers {
		healthchecker.AddCheck(cd.Name, true, healthchecker.HTTPChecker(cd.HealthcheckURL))
	}

	go healthchecker.RunCheckers(15)

	go runHTTPServer(ctx, conf, healthchecker)

	<-sigchan
	log.Printf("Exiting Cleanly")
}

func makeTLSConfigWithCA(caCert []byte) (*tls.Config, error) {
	caCertPool, _ := x509.SystemCertPool()
	if caCertPool == nil {
		caCertPool = x509.NewCertPool()
	}
	if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
		return nil, errors.New("unable to parse PEM for CA Certificate")
	}

	tlsConfig := tls.Config{
		RootCAs: caCertPool,
	}

	return &tlsConfig, nil
}
