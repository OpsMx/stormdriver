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
	"flag"
	"log"

	"github.com/skandragon/gohealthcheck/health"
)

var (
	configFile    = flag.String("configFile", "/app/config/stormdriver.yaml", "Configuration file location")
	debug         = flag.Bool("debug", false, "enable debugging")
	conf          *configuration
	healthchecker = health.MakeHealth()
)

func main() {
	flag.Parse()

	conf = loadConfigurationFile(*configFile)

	if len(conf.Clouddrivers) == 0 {
		log.Printf("ERROR: no clouddrivers defined in config")
	}

	for _, cd := range conf.Clouddrivers {
		log.Printf("Clouddriver name: %s", cd.Name)
	}

	// make sure we have updated before we run the HTTP server.
	updateAccounts()

	go accountTracker()

	for _, cd := range conf.Clouddrivers {
		healthchecker.AddCheck(cd.Name, true, healthchecker.HTTPChecker(cd.HealthcheckURL))
	}

	go healthchecker.RunCheckers(15)

	runHTTPServer(conf, healthchecker)
}
