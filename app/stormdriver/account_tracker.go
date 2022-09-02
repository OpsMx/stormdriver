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
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/OpsMx/go-app-base/birger"
	"github.com/OpsMx/go-app-base/httputil"
)

type trackedSpinnakerAccount struct {
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	Type string `json:"type,omitempty" yaml:"type,omitempty"`
}

type trackedClouddriver struct {
	Source                  string    `json:"source,omitempty" yaml:"source,omitempty"`
	Name                    string    `json:"name,omitempty" yaml:"name,omitempty"`
	URL                     string    `json:"url,omitempty" yaml:"url,omitempty"`
	UIUrl                   string    `json:"uiUrl,omitempty" yaml:"uiUrl,omitempty"`
	AgentName               string    `json:"agentName,omitempty" yaml:"agentName,omitempty"`
	LastSuccessfulContact   time.Time `json:"lastSuccessfulContact,omitempty" yaml:"lastSuccessfulContact,omitempty"`
	Priority                int       `json:"priority,omitempty" yaml:"priority,omitempty"`
	DisableArtifactAccounts bool      `json:"disableArtifactAccounts,omitempty" yaml:"disableArtifactAccounts,omitempty"`
	healthcheckURL          string
	token                   string
}

const credentialsUpdateFrequency = 10

type ClouddriverManager struct {
	sync.Mutex

	// cloudAccountRoutes holds a mapping from account name (which is assumed to be
	// globally unique) to a specific clouddriver instance.  Use getKnownAccountRoutes()
	// or findAccountRoute() to read this; don't do it directly.
	cloudAccountRoutes map[string]URLAndPriority

	// cloudAccounts holds the list of all known spinnaker accounts.
	// use getKnownSpinnakerAccounts() to read this.  The contents of this
	// list may be entirely replaced, but the individual elements are immutable
	// when returned by that func.
	cloudAccounts []trackedSpinnakerAccount

	// same for artifacts
	artifactAccountRoutes map[string]URLAndPriority
	artifactAccounts      []trackedSpinnakerAccount

	state map[string]trackedClouddriver

	spinnakerUser string
}

func MakeClouddriverManager(clouddrivers []clouddriverConfig, spinnakerUser string) *ClouddriverManager {
	m := ClouddriverManager{
		spinnakerUser:         spinnakerUser,
		cloudAccountRoutes:    map[string]URLAndPriority{},
		cloudAccounts:         []trackedSpinnakerAccount{},
		artifactAccountRoutes: map[string]URLAndPriority{},
		artifactAccounts:      []trackedSpinnakerAccount{},
		state:                 map[string]trackedClouddriver{},
	}

	for _, clouddriver := range clouddrivers {
		key, tracked := makeTrackedClouddriverFromConfig(clouddriver)
		m.state[key] = tracked
	}
	return &m
}

func makeTrackedClouddriverFromConfig(clouddriver clouddriverConfig) (string, trackedClouddriver) {
	key := "config:" + clouddriver.Name
	ret := trackedClouddriver{
		Source:                  "config",
		Name:                    clouddriver.Name,
		URL:                     clouddriver.URL,
		UIUrl:                   clouddriver.UIUrl,
		LastSuccessfulContact:   time.Unix(0, 0).UTC(),
		DisableArtifactAccounts: clouddriver.DisableArtifactAccounts,
		Priority:                clouddriver.Priority,
	}
	healthchecker.AddCheck("argo "+key, true, &ret)

	return key, ret
}

func (a *trackedClouddriver) Check() error {
	code, _, err := fetchHealthcheck(context.Background(), a.token, a.healthcheckURL)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("healthcheck returned status %d", code)
	}
	return nil
}

func fetchHealthcheck(ctx context.Context, token string, url string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return http.StatusInternalServerError, []byte{}, err
	}
	if token != "" {
		req.Header.Set("authorization", fmt.Sprintf("Bearer %s", token))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return http.StatusUnprocessableEntity, []byte{}, err
	}
	defer resp.Body.Close()
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return http.StatusInternalServerError, []byte{}, err
	}
	return resp.StatusCode, content, nil
}

func (m *ClouddriverManager) accountTracker(updateChan chan birger.ServiceUpdate) {
	t := time.NewTimer(1 * time.Hour)
	t.Stop()

	m.updateAllAccounts(t)

	for {
		select {
		case update := <-updateChan:
			m.handleUpdate(update)
		case <-t.C:
			go m.updateAllAccounts(t)
		}
	}
}

func (m *ClouddriverManager) updateAllAccounts(t *time.Timer) {
	ctx, span := tracerProvider.Provider.Tracer("updateAllAccounts").Start(context.Background(), "updateAllAccounts")
	defer span.End()

	log.Printf("Updating all accounts")

	var wg sync.WaitGroup
	wg.Add(2)
	go m.updateAccounts(ctx, &wg)
	go m.updateArtifactAccounts(ctx, &wg)
	wg.Wait()
	t.Reset(credentialsUpdateFrequency * time.Second)
}

func (m *ClouddriverManager) handleUpdate(update birger.ServiceUpdate) {
	log.Printf("Got update: %v", update)
}

func copyRoutes(src map[string]URLAndPriority) map[string]URLAndPriority {
	ret := make(map[string]URLAndPriority, len(src))
	for name, cd := range src {
		ret[name] = cd
	}
	return ret
}

func (m *ClouddriverManager) getCloudAccountRoutes() map[string]URLAndPriority {
	m.Lock()
	defer m.Unlock()
	return copyRoutes(m.cloudAccountRoutes)
}

func (m *ClouddriverManager) getArtifactAccountRoutes() map[string]URLAndPriority {
	m.Lock()
	defer m.Unlock()
	return copyRoutes(m.artifactAccountRoutes)
}

func copyTrackedAccounts(src []trackedSpinnakerAccount) []trackedSpinnakerAccount {
	ret := make([]trackedSpinnakerAccount, len(src))
	copy(ret, src)
	return ret
}

func (m *ClouddriverManager) getCloudAccounts() []trackedSpinnakerAccount {
	m.Lock()
	defer m.Unlock()
	return copyTrackedAccounts(m.cloudAccounts)
}

func (m *ClouddriverManager) getArtifactAccounts() []trackedSpinnakerAccount {
	m.Lock()
	defer m.Unlock()
	return copyTrackedAccounts(m.artifactAccounts)
}

func (m *ClouddriverManager) findCloudRoute(name string) (string, bool) {
	m.Lock()
	defer m.Unlock()
	val, found := m.cloudAccountRoutes[name]
	return val.URL, found
}

func (m *ClouddriverManager) findArtifactRoute(name string) (string, bool) {
	m.Lock()
	defer m.Unlock()
	val, found := m.artifactAccountRoutes[name]
	return val.URL, found
}

func (m *ClouddriverManager) getHealthyClouddriverURLs() []string {
	m.Lock()
	defer m.Unlock()
	healthy := map[string]bool{}
	for _, v := range m.cloudAccountRoutes {
		healthy[v.URL] = true
	}
	for _, v := range m.artifactAccountRoutes {
		healthy[v.URL] = true
	}
	return keysForMapStringToBool(healthy)
}

func (m *ClouddriverManager) getClouddriverURLs(artifactAccount bool) []URLAndPriority {
	ret := []URLAndPriority{}
	for _, cd := range m.state {
		if !artifactAccount || (artifactAccount && !cd.DisableArtifactAccounts) {
			ret = append(ret, URLAndPriority{cd.URL, cd.Priority})
		}
	}
	return ret
}

func (m *ClouddriverManager) updateAccounts(ctx context.Context, wg *sync.WaitGroup) {
	m.Lock()
	defer m.Unlock()
	defer wg.Done()
	ctx, span := tracerProvider.Provider.Tracer("updateAccounts").Start(ctx, "updateAccounts")
	defer span.End()
	cds := m.getClouddriverURLs(false)
	newAccountRoutes, newAccounts := fetchCreds(ctx, cds, "/credentials", m.spinnakerUser)

	m.cloudAccountRoutes = newAccountRoutes
	m.cloudAccounts = newAccounts
}

func (m *ClouddriverManager) updateArtifactAccounts(ctx context.Context, wg *sync.WaitGroup) {
	m.Lock()
	defer m.Unlock()
	defer wg.Done()
	ctx, span := tracerProvider.Provider.Tracer("updateArtifactAccounts").Start(ctx, "updateArtifactAccounts")
	defer span.End()
	cds := m.getClouddriverURLs(true)
	newAccountRoutes, newAccounts := fetchCreds(ctx, cds, "/artifacts/credentials", m.spinnakerUser)

	m.artifactAccountRoutes = newAccountRoutes
	m.artifactAccounts = newAccounts
}

type credentialsResponse struct {
	accounts []trackedSpinnakerAccount
	url      string
	priority int
}

func fetchCredsFromOne(ctx context.Context, c chan credentialsResponse, cd URLAndPriority, path string, headers http.Header) {
	resp := credentialsResponse{url: cd.URL, priority: cd.Priority}
	fullURL := combineURL(cd.URL, path)
	data, code, _, err := fetchGet(ctx, fullURL, headers)
	if err != nil {
		log.Printf("Unable to fetch credentials from %s: %v", fullURL, err)
		c <- resp
		return
	}

	if !httputil.StatusCodeOK(code) {
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

func fetchCreds(ctx context.Context, cds []URLAndPriority, path string, spinnakerUser string) (map[string]URLAndPriority, []trackedSpinnakerAccount) {
	newAccountRoutes := map[string]URLAndPriority{}
	newAccounts := []trackedSpinnakerAccount{}

	headers := http.Header{}
	headers.Set("x-spinnaker-user", spinnakerUser)
	headers.Set("accept", "*/*")

	c := make(chan credentialsResponse, len(cds))
	for _, cd := range cds {
		go fetchCredsFromOne(ctx, c, cd, path, headers)
	}
	for i := 0; i < len(cds); i++ {
		creds := <-c
		newAccounts = mergeIfUnique(URLAndPriority{creds.url, creds.priority}, creds.accounts, newAccountRoutes, newAccounts)
	}

	return newAccountRoutes, newAccounts
}

func mergeIfUnique(cd URLAndPriority, instanceAccounts []trackedSpinnakerAccount, routes map[string]URLAndPriority, newAccounts []trackedSpinnakerAccount) []trackedSpinnakerAccount {
	for _, account := range instanceAccounts {
		current, seen := routes[account.Name]
		if !seen {
			routes[account.Name] = cd
			newAccounts = append(newAccounts, account)
		} else if current.Priority < cd.Priority {
			routes[account.Name] = cd
		}
	}
	return newAccounts
}
