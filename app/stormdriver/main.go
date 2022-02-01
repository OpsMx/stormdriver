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
	"flag"
	"log"
	"os"
	"time"

	"github.com/skandragon/gohealthcheck/health"

	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
)

var (
	configFile = flag.String("configFile", "/app/config/stormdriver.yaml", "Configuration file location")

	// eg, http://localhost:14268/api/traces
	jaegerEndpoint = flag.String("jaeger-endpoint", "", "Jaeger collector endpoint")

	debug         = flag.Bool("debug", false, "enable debugging")
	conf          *configuration
	healthchecker = health.MakeHealth()
)

func showGitInfo() {
	gitBranch := os.Getenv("GIT_BRANCH")
	if gitBranch == "" {
		gitBranch = "dev"
	}
	gitHash := os.Getenv("GIT_HASH")
	if gitHash == "" {
		gitHash = "dev"
	}
	log.Printf("GIT Version: %s @ %s", gitBranch, gitHash)
}

func main() {
	showGitInfo()

	flag.Parse()

	tp, err := tracerProvider(*jaegerEndpoint)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func(ctx context.Context) {

		ctx, cancel = context.WithTimeout(ctx, time.Second*5)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			log.Fatal(err)
		}
	}(ctx)

	tr := tp.Tracer("main")
	ctx, span := tr.Start(ctx, "main")
	defer span.End()

	conf = loadConfigurationFile(*configFile)

	if len(conf.Clouddrivers) == 0 {
		log.Printf("ERROR: no clouddrivers defined in config")
	}

	for _, cd := range conf.Clouddrivers {
		log.Printf("Clouddriver name: %s", cd.Name)
	}

	// make sure we have updated before we run the HTTP server.
	updateAllAccounts()

	go accountTracker()

	for _, cd := range conf.Clouddrivers {
		healthchecker.AddCheck(cd.Name, true, healthchecker.HTTPChecker(cd.HealthcheckURL))
	}

	go healthchecker.RunCheckers(15)

	runHTTPServer(ctx, conf, healthchecker)
}

// tracerProvider returns an OpenTelemetry TracerProvider configured to use
// the Jaeger exporter that will send spans to the provided url. The returned
// TracerProvider will also use a Resource configured with all the information
// about the application.
func tracerProvider(url string) (*tracesdk.TracerProvider, error) {
	opts := []tracesdk.TracerProviderOption{
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("stormdriver"),
		)),
	}
	if url != "" {
		exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(url)))
		if err != nil {
			return nil, err
		}
		opts = append(opts, tracesdk.WithBatcher(exp))
	}
	tp := tracesdk.NewTracerProvider(opts...)
	return tp, nil
}
