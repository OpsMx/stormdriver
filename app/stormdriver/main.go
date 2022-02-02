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
	"os/signal"
	"syscall"
	"time"

	"github.com/skandragon/gohealthcheck/health"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

var (
	configFile = flag.String("configFile", "/app/config/stormdriver.yaml", "Configuration file location")

	// eg, http://localhost:14268/api/traces
	jaegerEndpoint = flag.String("jaeger-endpoint", "", "Jaeger collector endpoint")

	debug         = flag.Bool("debug", false, "enable debugging")
	conf          *configuration
	healthchecker = health.MakeHealth()
	tracer        trace.Tracer
)

func getEnvar(name string, defaultValue string) string {
	value, found := os.LookupEnv(name)
	if !found {
		return defaultValue
	}
	return value
}

func gitBranch() string {
	return getEnvar("GIT_BRANCH", "dev")
}

func gitHash() string {
	return getEnvar("GIT_HASH", "dev")
}

func showGitInfo() {
	log.Printf("GIT Version: %s @ %s", gitBranch(), gitHash())
}

func main() {
	showGitInfo()

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGTERM, syscall.SIGINT)

	flag.Parse()
	if len(*jaegerEndpoint) == 0 {
		*jaegerEndpoint = getEnvar("JAEGER_TRACE_URL", "")
	}

	tracerProvider, err := newTracerProvider(*jaegerEndpoint, gitHash())
	if err != nil {
		log.Fatal(err)
	}
	otel.SetTracerProvider(tracerProvider)
	tracer = tracerProvider.Tracer("main")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func(ctx context.Context) {
		ctx, cancel = context.WithTimeout(ctx, time.Second*5)
		defer cancel()
		if err := tracerProvider.Shutdown(ctx); err != nil {
			log.Fatal(err)
		}
	}(ctx)

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

	go runHTTPServer(ctx, conf, healthchecker)

	<-sigchan
	log.Printf("Exiting Cleanly")
}
