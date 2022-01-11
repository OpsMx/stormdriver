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

	"gopkg.in/yaml.v3"
)

const defaultHTTPListenPort = 8090
const defaultDialTimeout = 15
const defaultClientTimeout = 15
const defaultTLSHandshakeTimeout = 15
const defaultResponseHeaderTimeout = 15
const defaultMaxIdleConns = 5
const defaultSpinnakerUser = "anonymous"

type clouddriverConfig struct {
	URL string `yaml:"url,omitempty"`
}

type configuration struct {
	HTTPListenPort        uint16              `yaml:"httpListenPort,omitempty"`
	DialTimeout           int                 `yaml:"dialTimeout,omitempty"`
	ClientTimeout         int                 `yaml:"clientTimeout,omitempty"`
	TLSHandshakeTimeout   int                 `yaml:"tlsHandshakeTimeout,omitempty"`
	ResponseHeaderTimeout int                 `yaml:"responseHeaderTimeout,omitempty"`
	MaxIdleConnections    int                 `yaml:"maxIdleConnections,omitempty"`
	SpinnakerUser         string              `yaml:"spinnakerUser,omitempty"`
	Clouddrivers          []clouddriverConfig `yaml:"clouddrivers,omitempty"`
}

func (c *configuration) applyDefaults() {
	if c.HTTPListenPort == 0 {
		c.HTTPListenPort = defaultHTTPListenPort
	}
	if c.DialTimeout == 0 {
		c.DialTimeout = defaultDialTimeout
	}
	if c.ClientTimeout == 0 {
		c.ClientTimeout = defaultClientTimeout
	}
	if c.TLSHandshakeTimeout == 0 {
		c.TLSHandshakeTimeout = defaultTLSHandshakeTimeout
	}
	if c.ResponseHeaderTimeout == 0 {
		c.ResponseHeaderTimeout = defaultResponseHeaderTimeout
	}
	if c.MaxIdleConnections == 0 {
		c.MaxIdleConnections = defaultMaxIdleConns
	}
	if c.Clouddrivers == nil {
		c.Clouddrivers = []clouddriverConfig{}
	}
	if c.SpinnakerUser == "" {
		c.SpinnakerUser = defaultSpinnakerUser
	}
}

func (c *configuration) validate() error {
	for idx, cm := range c.Clouddrivers {
		if cm.URL == "" {
			return fmt.Errorf("clouddriver index %d missing url", idx+1)
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
