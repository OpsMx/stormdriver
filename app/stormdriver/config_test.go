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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ParseFile(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		wantOut     *configuration
		expectError bool
	}{
		{
			"empty sets defaults",
			[]byte(``),
			&configuration{
				HTTPListenPort:        defaultHTTPListenPort,
				DialTimeout:           defaultDialTimeout,
				ClientTimeout:         defaultClientTimeout,
				TLSHandshakeTimeout:   defaultTLSHandshakeTimeout,
				ResponseHeaderTimeout: defaultResponseHeaderTimeout,
				MaxIdleConnections:    defaultMaxIdleConns,
				SpinnakerUser:         defaultSpinnakerUser,
				Clouddrivers:          []clouddriverConfig{},
			},
			false,
		},
		{
			"defaults do not override integer",
			[]byte(`httpListenPort: 1234`),
			&configuration{
				HTTPListenPort:        1234,
				DialTimeout:           defaultDialTimeout,
				ClientTimeout:         defaultClientTimeout,
				TLSHandshakeTimeout:   defaultTLSHandshakeTimeout,
				ResponseHeaderTimeout: defaultResponseHeaderTimeout,
				MaxIdleConnections:    defaultMaxIdleConns,
				SpinnakerUser:         defaultSpinnakerUser,
				Clouddrivers:          []clouddriverConfig{},
			},
			false,
		},
		{
			"defaults do not override string",
			[]byte(`spinnakerUser: michael`),
			&configuration{
				HTTPListenPort:        defaultHTTPListenPort,
				DialTimeout:           defaultDialTimeout,
				ClientTimeout:         defaultClientTimeout,
				TLSHandshakeTimeout:   defaultTLSHandshakeTimeout,
				ResponseHeaderTimeout: defaultResponseHeaderTimeout,
				MaxIdleConnections:    defaultMaxIdleConns,
				SpinnakerUser:         "michael",
				Clouddrivers:          []clouddriverConfig{},
			},
			false,
		},
		{
			"config parses with clouddrivers",
			[]byte(`clouddrivers:
  - url: abcd
  - url: wxyz`),
			&configuration{
				HTTPListenPort:        defaultHTTPListenPort,
				DialTimeout:           defaultDialTimeout,
				ClientTimeout:         defaultClientTimeout,
				TLSHandshakeTimeout:   defaultTLSHandshakeTimeout,
				ResponseHeaderTimeout: defaultResponseHeaderTimeout,
				MaxIdleConnections:    defaultMaxIdleConns,
				SpinnakerUser:         defaultSpinnakerUser,
				Clouddrivers: []clouddriverConfig{
					{URL: "abcd"},
					{URL: "wxyz"},
				},
			},
			false,
		},
		{
			"fails with a blank 'url' for clouddriver",
			[]byte(`clouddrivers:
  - URL: abcd`),
			&configuration{},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := loadConfiguration(tt.input)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, actual)
				assert.Equal(t, tt.wantOut, actual)
			}
		})
	}
}
