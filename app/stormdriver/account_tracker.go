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
	// spinnakerAccountRoutes holds a mapping from account name (which is assumed to be
	// globally unique) to a specific clouddriver instance.  Use getKnownAccountRoutes()
	// or findAccountRoute() to read this; don't do it directly.
	spinnakerAccountRoutes map[string]string

	// spinnakerAccounts holds the list of all known spinnaker accounts.
	// use getKnownSpinnakerAccounts() to read this.  The contents of this
	// list may be entirely replaced, but the individual elements are immutable
	// when returned by that func.
	spinnakerAccounts []trackedSpinnakerAccount

	knownAccountsLock sync.Mutex
)

func accountTracker() {
	for {
		time.Sleep(10 * time.Second)

		updateAccounts()
	}
}

func getKnownAccountRoutes() map[string]string {
	knownAccountsLock.Lock()
	defer knownAccountsLock.Unlock()
	ret := make(map[string]string, len(spinnakerAccountRoutes))
	for name, url := range spinnakerAccountRoutes {
		ret[name] = url
	}
	return ret
}

func getKnownSpinnakerAccounts() []trackedSpinnakerAccount {
	knownAccountsLock.Lock()
	defer knownAccountsLock.Unlock()
	ret := make([]trackedSpinnakerAccount, len(spinnakerAccounts))
	for idx, account := range spinnakerAccounts {
		ret[idx] = account
	}
	return ret
}

func findAccountRoute(name string) (string, bool) {
	knownAccountsLock.Lock()
	defer knownAccountsLock.Unlock()
	val, found := spinnakerAccountRoutes[name]
	return val, found
}

func updateAccounts() {
	urls := conf.getClouddriverURLs()

	headers := http.Header{}
	headers.Set("x-spinnaker-user", conf.SpinnakerUser)

	newAccountRoutes := map[string]string{}
	newAccounts := []trackedSpinnakerAccount{}

	for _, url := range urls {
		data, code, _, err := fetchGet(combineURL(url, "/credentials"), headers)
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

		for _, account := range instanceAccounts {
			newAccountRoutes[account.Name] = url
			newAccounts = append(newAccounts, account)
		}
	}

	knownAccountsLock.Lock()
	defer knownAccountsLock.Unlock()
	spinnakerAccountRoutes = newAccountRoutes
	spinnakerAccounts = newAccounts
}
