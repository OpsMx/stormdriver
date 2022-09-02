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
	"fmt"
	"log"
	"net/url"
	"os"

	"github.com/OpsMx/go-app-base/birger"
	"github.com/OpsMx/go-app-base/httputil"
	"gopkg.in/yaml.v3"
)

const defaultHTTPListenPort = 7002
const defaultSpinnakerUser = "anonymous"

type clouddriverConfig struct {
	Name                    string `yaml:"name,omitempty" json:"name,omitempty"`
	URL                     string `yaml:"url,omitempty" json:"url,omitempty"`
	HealthcheckURL          string `yaml:"healthcheckUrl,omitempty" json:"healthcheckUrl,omitempty"`
	DisableArtifactAccounts bool   `yaml:"disableArtifactAccounts,omitempty" json:"disableArtifactAccounts,omitempty"`
	Priority                int    `yaml:"priority,omitempty" json:"priority,omitempty"`
	UIUrl                   string `json:"uiUrl,omitempty" yaml:"uiUrl,omitempty"`
}

type configuration struct {
	HTTPListenPort   uint16                `yaml:"httpListenPort,omitempty" json:"httpListenPort,omitempty"`
	HTTPClientConfig httputil.ClientConfig `json:"httpClientConfig,omitempty" yaml:"httpClientConfig,omitempty"`
	Controller       birger.Config         `json:"controller,omitempty" yaml:"controller,omitempty"`
	SpinnakerUser    string                `yaml:"spinnakerUser,omitempty" json:"spinnakerUser,omitempty"`
	Clouddrivers     []clouddriverConfig   `yaml:"clouddrivers,omitempty" json:"clouddrivers,omitempty"`
}

func (c *configuration) applyDefaults() {
	if c.HTTPListenPort == 0 {
		c.HTTPListenPort = defaultHTTPListenPort
	}
	httputil.SetClientConfig(c.HTTPClientConfig)
	if c.Clouddrivers == nil {
		c.Clouddrivers = []clouddriverConfig{}
	}
	if c.SpinnakerUser == "" {
		c.SpinnakerUser = defaultSpinnakerUser
	}

	if c.Clouddrivers == nil {
		c.Clouddrivers = []clouddriverConfig{}
	}

	for idx := 0; idx < len(c.Clouddrivers); idx++ {
		cd := &c.Clouddrivers[idx]
		if len(cd.Name) == 0 {
			cd.Name = fmt.Sprintf("clouddriver[%d]", idx)
		}
		if len(cd.HealthcheckURL) == 0 && len(cd.URL) != 0 {
			cd.HealthcheckURL = combineURL(cd.URL, "/health")
		}
	}
}

func (configuration) validateURL(u string) error {
	_, err := url.Parse(u)
	return err
}

func (c configuration) validate() error {
	for idx, cm := range c.Clouddrivers {
		if cm.URL == "" {
			return fmt.Errorf("clouddriver index %d missing url", idx+1)
		}
		err := c.validateURL(cm.URL)
		if err != nil {
			return fmt.Errorf("clouddriver index %d: malformed URL", idx+1)
		}
		err = c.validateURL(cm.HealthcheckURL)
		if err != nil {
			return fmt.Errorf("clouddriver index %d: malformed healthcheck URL", idx+1)
		}
	}
	return nil
}

func loadConfiguration(y []byte) (*configuration, error) {
	config := &configuration{}
	err := yaml.Unmarshal(y, config)
	if err != nil {
		return nil, err
	}

	config.applyDefaults()

	err = config.validate()
	if err != nil {
		return nil, err
	}

	return config, nil
}

func loadConfigurationFile(filename string) *configuration {
	buf, err := os.ReadFile(filename)
	if err != nil {
		log.Fatal(err)
	}

	config, err := loadConfiguration(buf)
	if err != nil {
		log.Fatal(err)
	}
	return config
}

// URLAndPriority holds the URL and current priority.
type URLAndPriority struct {
	URL      string `json:"url,omitempty"`
	Priority int    `json:"priority,omitempty"`
}
