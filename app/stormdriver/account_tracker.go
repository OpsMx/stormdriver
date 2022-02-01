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
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

type trackedSpinnakerAccount struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}

var (
	// cloudAccountRoutes holds a mapping from account name (which is assumed to be
	// globally unique) to a specific clouddriver instance.  Use getKnownAccountRoutes()
	// or findAccountRoute() to read this; don't do it directly.
	cloudAccountRoutes map[string]string

	// cloudAccounts holds the list of all known spinnaker accounts.
	// use getKnownSpinnakerAccounts() to read this.  The contents of this
	// list may be entirely replaced, but the individual elements are immutable
	// when returned by that func.
	cloudAccounts []trackedSpinnakerAccount

	// same for artifacts
	artifactAccountRoutes map[string]string
	artifactAccounts      []trackedSpinnakerAccount

	knownAccountsLock sync.Mutex
)

const credentialsUpdateFrequency = 10

func accountTracker() {
	for {
		time.Sleep(credentialsUpdateFrequency * time.Second)

		updateAllAccounts()
	}
}

func updateAllAccounts() {
	updateAccounts()
	updateArtifactAccounts()
}

func copyRoutes(src map[string]string) map[string]string {
	ret := make(map[string]string, len(src))
	for name, url := range src {
		ret[name] = url
	}
	return ret
}

func getCloudAccountRoutes() map[string]string {
	knownAccountsLock.Lock()
	defer knownAccountsLock.Unlock()
	return copyRoutes(cloudAccountRoutes)
}

func getArtifactAccountRoutes() map[string]string {
	knownAccountsLock.Lock()
	defer knownAccountsLock.Unlock()
	return copyRoutes(artifactAccountRoutes)
}

func copyTrackedAccounts(src []trackedSpinnakerAccount) []trackedSpinnakerAccount {
	ret := make([]trackedSpinnakerAccount, len(src))
	for idx, account := range src {
		ret[idx] = account
	}
	return ret
}

func getCloudAccounts() []trackedSpinnakerAccount {
	knownAccountsLock.Lock()
	defer knownAccountsLock.Unlock()
	return copyTrackedAccounts(cloudAccounts)
}

func getArtifactAccounts() []trackedSpinnakerAccount {
	knownAccountsLock.Lock()
	defer knownAccountsLock.Unlock()
	return copyTrackedAccounts(artifactAccounts)
}

func findCloudRoute(name string) (string, bool) {
	knownAccountsLock.Lock()
	defer knownAccountsLock.Unlock()
	val, found := cloudAccountRoutes[name]
	return val, found
}

func findArtifactRoute(name string) (string, bool) {
	knownAccountsLock.Lock()
	defer knownAccountsLock.Unlock()
	val, found := artifactAccountRoutes[name]
	return val, found
}

func getHealthyClouddriverURLs() []string {
	knownAccountsLock.Lock()
	defer knownAccountsLock.Unlock()
	healthy := map[string]bool{}
	for _, v := range cloudAccountRoutes {
		healthy[v] = true
	}
	return keysForMapStringToBool(healthy)
}

func updateAccounts() {
	urls := conf.getClouddriverURLs()
	newAccountRoutes, newAccounts := fetchCreds(urls, "/credentials")

	knownAccountsLock.Lock()
	defer knownAccountsLock.Unlock()
	cloudAccountRoutes = newAccountRoutes
	cloudAccounts = newAccounts
}

func updateArtifactAccounts() {
	urls := conf.getClouddriverURLs()
	newAccountRoutes, newAccounts := fetchCreds(urls, "/artifacts/credentials")

	knownAccountsLock.Lock()
	defer knownAccountsLock.Unlock()
	artifactAccountRoutes = newAccountRoutes
	artifactAccounts = newAccounts
}

func fetchCreds(urls []string, path string) (map[string]string, []trackedSpinnakerAccount) {
	newAccountRoutes := map[string]string{}
	newAccounts := []trackedSpinnakerAccount{}

	headers := http.Header{}
	headers.Set("x-spinnaker-user", conf.SpinnakerUser)

	for _, url := range urls {
		data, code, _, err := fetchGet(combineURL(url, path), headers)
		if err != nil {
			log.Printf("Unable to fetch credentials from %s: %v", url, err)
			continue
		}
		if !statusCodeOK(code) {
			log.Printf("Unable to fetch credentials from %s: status %d", url, code)
			continue
		}

		var instanceAccounts []trackedSpinnakerAccount
		err = json.Unmarshal(data, &instanceAccounts)
		if err != nil {
			log.Printf("Unable to parse response for credentials from %s: %v", url, err)
			continue
		}

		newAccounts = mergeIfUnique(url, instanceAccounts, newAccountRoutes, newAccounts)
	}

	return newAccountRoutes, newAccounts
}

func mergeIfUnique(url string, instanceAccounts []trackedSpinnakerAccount, routes map[string]string, newAccounts []trackedSpinnakerAccount) []trackedSpinnakerAccount {
	for _, account := range instanceAccounts {
		if _, seen := routes[account.Name]; !seen {
			routes[account.Name] = url
			newAccounts = append(newAccounts, account)
		}
	}
	return newAccounts
}
