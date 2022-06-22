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
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
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
	ctx, span := tracer.Start(context.Background(), "updateAllAccounts")
	span.SetAttributes(attribute.String("otel.library.name", "account_tracker"))
	defer span.End()

	var wg sync.WaitGroup
	wg.Add(2)
	go updateAccounts(ctx, &wg)
	go updateArtifactAccounts(ctx, &wg)
	wg.Wait()
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
	for _, v := range artifactAccountRoutes {
		healthy[v] = true
	}
	return keysForMapStringToBool(healthy)
}

func updateAccounts(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	ctx, span := tracer.Start(ctx, "updateAccounts")
	defer span.End()
	urls := conf.getClouddriverURLs()
	newAccountRoutes, newAccounts := fetchCreds(ctx, urls, "/credentials")

	knownAccountsLock.Lock()
	defer knownAccountsLock.Unlock()
	cloudAccountRoutes = newAccountRoutes
	cloudAccounts = newAccounts
}

func updateArtifactAccounts(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	ctx, span := tracer.Start(ctx, "updateArtifactAccounts")
	defer span.End()
	urls := conf.getClouddriverURLs()
	newAccountRoutes, newAccounts := fetchCreds(ctx, urls, "/artifacts/credentials")

	knownAccountsLock.Lock()
	defer knownAccountsLock.Unlock()
	artifactAccountRoutes = newAccountRoutes
	artifactAccounts = newAccounts
}

type credentialsResponse struct {
	accounts []trackedSpinnakerAccount
	url      string
}

func fetchCredsFromOne(ctx context.Context, c chan credentialsResponse, url string, path string, headers http.Header) {
	resp := credentialsResponse{url: url}
	fullURL := combineURL(url, path)
	data, code, _, err := fetchGet(ctx, fullURL, headers)
	if err != nil {
		log.Printf("Unable to fetch credentials from %s: %v", fullURL, err)
		c <- resp
		return
	}

	if !statusCodeOK(code) {
		log.Printf("Unable to fetch credentials from %s: status %d", fullURL, code)
		c <- resp
		return
	}

	var instanceAccounts []trackedSpinnakerAccount
	err = json.Unmarshal(data, &instanceAccounts)
	if err != nil {
		log.Printf("Unable to parse response for credentials from %s: %v", fullURL, err)
		c <- resp
		return
	}
	resp.accounts = instanceAccounts
	c <- resp
}

func fetchCreds(ctx context.Context, urls []string, path string) (map[string]string, []trackedSpinnakerAccount) {
	newAccountRoutes := map[string]string{}
	newAccounts := []trackedSpinnakerAccount{}

	headers := http.Header{}
	headers.Set("x-spinnaker-user", conf.SpinnakerUser)
	headers.Set("accept", "*/*")

	c := make(chan credentialsResponse, len(urls))
	for _, url := range urls {
		go fetchCredsFromOne(ctx, c, url, path, headers)
	}
	for i := 0; i < len(urls); i++ {
		creds := <-c
		newAccounts = mergeIfUnique(creds.url, creds.accounts, newAccountRoutes, newAccounts)
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
