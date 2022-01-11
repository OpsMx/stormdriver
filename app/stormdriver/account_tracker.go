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

var (
	knownAccounts     map[string]string
	knownAccountsLock sync.Mutex
)

func accountTracker() {
	for {
		time.Sleep(10 * time.Second)

		updateAccounts()
	}
}

func getAccountRoutes() map[string]string {
	knownAccountsLock.Lock()
	defer knownAccountsLock.Unlock()
	ret := make(map[string]string, len(knownAccounts))
	for name, url := range knownAccounts {
		ret[name] = url
	}
	return ret
}

func findAccountRoute(name string) (string, bool) {
	knownAccountsLock.Lock()
	defer knownAccountsLock.Unlock()
	val, found := knownAccounts[name]
	return val, found
}

type accountWithName struct {
	Name string `json:"name,omitempty"`
}

func updateAccounts() {
	urls := getClouddriverURLs()

	//log.Printf("Updating accounts from: %v", urls)

	newList := make(map[string]string)
	for _, url := range urls {
		//log.Printf("Updating accounts from: %s", url)
		data, code, err := fetchGet(combineURL(url, "/credentials"), http.Header{})
		if err != nil {
			log.Printf("Unable to fetch credentials from %s: %v", url, err)
			continue
		}
		if !statusCodeOK(code) {
			log.Printf("Unable to fetch credentials from %s: status %d", url, code)
			continue
		}

		names := []accountWithName{}
		err = json.Unmarshal(data, &names)
		if err != nil {
			log.Printf("Unable to parse response for credentials from %s: %v", url, err)
			continue
		}

		for _, name := range names {
			newList[name.Name] = url
		}
	}

	knownAccountsLock.Lock()
	defer knownAccountsLock.Unlock()
	knownAccounts = newList
}
