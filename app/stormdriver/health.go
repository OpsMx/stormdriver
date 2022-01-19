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
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// HealthChecker is an interface that defines a Check() function.  This Check()
// will be called periorically from a goproc, so if any external resources
// need to be locked, it must handle this correctly.
// It should return an error if the check fails, where the contents of the
// error will be included in the health indicator's JSON.
// Return nil to indicate success.
type HealthChecker interface {
	Check() error
}

type httpChecker struct {
	url string
}

type healthIndicator struct {
	Service     string `json:"service,omitempty"`
	Healthy     bool   `json:"healthy,omitempty"`
	Message     string `json:"message,omitempty"`
	ObserveOnly bool   `json:"observeOnly,omitempty"`
	LastChecked uint64 `json:"lastChecked,omitempty"`

	checker HealthChecker
}

type health struct {
	sync.Mutex
	run bool

	Healthy bool `json:"healthy,omitempty"`

	Checks []healthIndicator `json:"checks,omitempty"`
}

func makeHealth() *health {
	return &health{
		Healthy: true,
	}
}

func removeChecker(s []healthIndicator, i int) []healthIndicator {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}

// AddCheck adds a new checker.  For HTTP checkers, use health.HTTPChecker(url).
func (h *health) AddCheck(service string, observeOnly bool, checker HealthChecker) {
	h.Lock()
	defer h.Unlock()
	for _, c := range h.Checks {
		if c.Service == service {
			c.checker = checker
			c.ObserveOnly = observeOnly
			return
		}
	}
	h.Checks = append(h.Checks, healthIndicator{service, true, "", observeOnly, 0, checker})
}

// RemoveCheck removes a checker.  This will eventually converge in the output.
func (h *health) RemoveCheck(service string) {
	h.Lock()
	defer h.Unlock()
	for idx, c := range h.Checks {
		if c.Service == service {
			h.Checks = removeChecker(h.Checks, idx)
			return
		}
	}
}

func (h *health) runChecker(checker healthIndicator) {
	err := checker.checker.Check()
	if err == nil {
		checker.Healthy = true
		checker.Message = "OK"
		checker.LastChecked = uint64(time.Now().UnixMilli())
	} else {
		checker.Healthy = false
		checker.Message = fmt.Sprintf("%s ERROR %v", checker.Service, err)
		checker.LastChecked = uint64(time.Now().UnixMilli())
	}
}

// RunCheckers runs all the health checks, one every frequency/count seconds.
func (h *health) RunCheckers(frequency int) {
	nextIndex := 0

	h.Lock()
	count := len(h.Checks)
	h.Unlock()

	for {
		// ensure we sleep while not locked.
		sleepDuration := time.Duration(frequency) * time.Second / time.Duration(count)
		time.Sleep(sleepDuration)

		// locked while manitulating things and calling healthcheck
		h.Lock()
		count = len(h.Checks)
		if !h.run {
			h.Unlock()
			return
		}
		if nextIndex >= len(h.Checks) {
			nextIndex = 0
		}
		h.Unlock()
		h.runChecker(h.Checks[nextIndex])
		h.Lock()
		nextIndex++

		// Now, check all statuses and compute the global status
		h.Healthy = true
		for _, c := range h.Checks {
			if c.ObserveOnly {
				continue
			}
			h.Healthy = h.Healthy && c.Healthy
		}
		h.Unlock()
	}
}

// StopCheckers will stop running RunCheckers()
func (h *health) StopCheckers() {
	h.Lock()
	defer h.Unlock()
	h.run = false
}

func (h *health) HTTPChecker(url string) HealthChecker {
	return httpChecker{url: url}
}

// HTTP handler which returns 200 if all critical checks pass, or 500 if not.
func (h *health) HTTPHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")
	h.Lock()
	data, err := json.Marshal(h)
	healthy := h.Healthy
	h.Unlock()
	if err != nil {
		w.WriteHeader(500)
		log.Printf("Healthcheck HTTPHandler: %v", err)
		return
	}
	if healthy {
		w.WriteHeader(200)
	} else {
		w.WriteHeader(418)
	}
	w.Write(data)
}

// Check implements the HealthChecker interface, using a HTTP fetch.
// Any status code between 200 and 399 indicates success, any other
// indicates a failure.
func (hc httpChecker) Check() error {
	resp, err := http.Get(hc.url)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return nil
	}
	return fmt.Errorf("HTTP status code %d returned", resp.StatusCode)
}
