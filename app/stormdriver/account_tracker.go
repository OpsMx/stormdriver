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
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
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
	artifactHealth          error
	accountHealth           error
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

	state map[string]*trackedClouddriver

	spinnakerUser string
	health        error
}

func MakeClouddriverManager(clouddrivers []clouddriverConfig, spinnakerUser string) *ClouddriverManager {
	m := ClouddriverManager{
		spinnakerUser:         spinnakerUser,
		cloudAccountRoutes:    map[string]URLAndPriority{},
		cloudAccounts:         []trackedSpinnakerAccount{},
		artifactAccountRoutes: map[string]URLAndPriority{},
		artifactAccounts:      []trackedSpinnakerAccount{},
		state:                 map[string]*trackedClouddriver{},
		health:                errors.New("initial sync not yet performed"),
	}

	for _, clouddriver := range clouddrivers {
		key, tracked := makeTrackedClouddriverFromConfig(clouddriver)
		m.state[key] = tracked
	}

	healthchecker.AddCheck("ClouddriverManager", true, &m)

	return &m
}

func (a *trackedClouddriver) Check() error {
	if a.artifactHealth != nil {
		return a.artifactHealth
	}
	return a.accountHealth
}

func (m *ClouddriverManager) accountTracker(updateChan chan birger.ServiceUpdate) {
	t := time.NewTimer(1 * time.Hour)
	t.Stop()

	m.updateAllAccounts(t)
	m.health = nil

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

func yesno(s string) bool {
	s = strings.ToLower(s)
	return s == "true" || s == "yes"
}

func makeTrackedClouddriverFromConfig(clouddriver clouddriverConfig) (string, *trackedClouddriver) {
	key := "config:" + clouddriver.Name
	healthcheck := clouddriver.HealthcheckURL
	if healthcheck == "" {
		healthcheck = clouddriver.URL + "/health"
	}
	var artifactHealth error = nil
	if !clouddriver.DisableArtifactAccounts {
		artifactHealth = errors.New("initial sync not yet performed")
	}
	ret := &trackedClouddriver{
		Source:                  "config",
		Name:                    clouddriver.Name,
		URL:                     clouddriver.URL,
		UIUrl:                   clouddriver.UIUrl,
		LastSuccessfulContact:   time.Unix(0, 0).UTC(),
		DisableArtifactAccounts: clouddriver.DisableArtifactAccounts,
		Priority:                clouddriver.Priority,
		healthcheckURL:          healthcheck,
		artifactHealth:          artifactHealth,
		accountHealth:           errors.New("initial sync not yet performed"),
	}
	healthchecker.AddCheck("clouddriver "+key, true, ret)

	return key, ret
}

func makeTrackedClouddriverFromUpdate(update birger.ServiceUpdate) *trackedClouddriver {
	uiUrl := update.Annotations["uiUrl"]
	disableArtifactAccounts := yesno(update.Annotations["disableArtifactAccounts"])
	priority := 0
	var err error
	if strpri := update.Annotations["priority"]; strpri != "" {
		if priority, err = strconv.Atoi(strpri); err != nil {
			log.Printf("WARNING: priority for %s from controller has bad priority: %s, using 0", update.Name, strpri)
		}
	}
	var artifactHealth error = nil
	if !disableArtifactAccounts {
		artifactHealth = errors.New("initial sync not yet performed")
	}
	return &trackedClouddriver{
		Source:                  "controller",
		Name:                    update.Name,
		URL:                     update.URL,
		UIUrl:                   uiUrl,
		LastSuccessfulContact:   time.Unix(0, 0).UTC(),
		AgentName:               update.AgentName,
		token:                   update.Token,
		DisableArtifactAccounts: disableArtifactAccounts,
		Priority:                priority,
		healthcheckURL:          update.URL + "/health",
		artifactHealth:          artifactHealth,
		accountHealth:           errors.New("initial sync not yet performed"),
	}
}

func (m *ClouddriverManager) handleUpdate(update birger.ServiceUpdate) {
	m.Lock()
	defer m.Unlock()

	key := "controller:" + update.AgentName + ":" + update.Name

	if update.Operation == "delete" {
		delete(m.state, key)
		healthchecker.RemoveCheck("clouddriver " + key)
		return
	}

	if update.Operation == "update" {
		old, found := m.state[key]
		tracked := makeTrackedClouddriverFromUpdate(update)
		if !found {
			m.state[key] = tracked
			healthchecker.AddCheck("clouddriver "+key, true, tracked)
			return
		}
		tracked.LastSuccessfulContact = old.LastSuccessfulContact
		healthchecker.RemoveCheck("clouddriver " + key)
		healthchecker.AddCheck("clouddriver "+key, true, tracked)
		m.state[key] = tracked
	}
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

func (m *ClouddriverManager) findCloudRoute(name string) (URLAndPriority, bool) {
	m.Lock()
	defer m.Unlock()
	val, found := m.cloudAccountRoutes[name]
	return val, found
}

func (m *ClouddriverManager) findArtifactRoute(name string) (URLAndPriority, bool) {
	m.Lock()
	defer m.Unlock()
	val, found := m.artifactAccountRoutes[name]
	return val, found
}

func (m *ClouddriverManager) getHealthyClouddriverURLs() []URLAndPriority {
	m.Lock()
	defer m.Unlock()
	healthy := map[string]URLAndPriority{}
	for _, v := range m.cloudAccountRoutes {
		healthy[v.key()] = v
	}
	for _, v := range m.artifactAccountRoutes {
		healthy[v.key()] = v
	}
	ret := []URLAndPriority{}
	for _, v := range healthy {
		ret = append(ret, v)
	}
	return ret
}

func (m *ClouddriverManager) getClouddriverURLs(artifactAccount bool) []URLAndPriority {
	ret := []URLAndPriority{}
	for _, cd := range m.state {
		if !artifactAccount || (artifactAccount && !cd.DisableArtifactAccounts) {
			ret = append(ret, URLAndPriority{cd.URL, cd.Priority, cd.token})
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
	cd       URLAndPriority
}

func fetchCredsFromOne(ctx context.Context, c chan credentialsResponse, cd URLAndPriority, path string, headers http.Header) {
	resp := credentialsResponse{cd: cd}
	fullURL := combineURL(cd.URL, path)
	data, code, _, err := fetchGet(ctx, fullURL, cd.token, headers)
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
		newAccounts = mergeIfUnique(creds.cd, creds.accounts, newAccountRoutes, newAccounts)
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

func (m *ClouddriverManager) Check() error {
	m.Lock()
	defer m.Unlock()
	return m.health
}
