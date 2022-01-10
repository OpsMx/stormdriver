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
		name    string
		input   []byte
		wantOut *configuration
	}{
		{
			"empty sets defaults",
			[]byte(``),
			&configuration{
				ListenPort:            defaultHTTPPort,
				DialTimeout:           defaultDialTimeout,
				ClientTimeout:         defaultClientTimeout,
				TLSHandshakeTimeout:   defaultTLSHandshakeTimeout,
				ResponseHeaderTimeout: defaultResponseHeaderTimeout,
				MaxIdleConnections:    defaultMaxIdleConns,
				Clouddrivers:          []clouddriverConfig{},
			},
		},
		{
			"defaults do not override settings",
			[]byte(`listenPort: 1234`),
			&configuration{
				ListenPort:            1234,
				DialTimeout:           defaultDialTimeout,
				ClientTimeout:         defaultClientTimeout,
				TLSHandshakeTimeout:   defaultTLSHandshakeTimeout,
				ResponseHeaderTimeout: defaultResponseHeaderTimeout,
				MaxIdleConnections:    defaultMaxIdleConns,
				Clouddrivers:          []clouddriverConfig{},
			},
		},
		{
			"config parses with clouddrivers",
			[]byte(`clouddrivers:
  - url: abcd
  - url: wxyz`),
			&configuration{
				ListenPort:            defaultHTTPPort,
				DialTimeout:           defaultDialTimeout,
				ClientTimeout:         defaultClientTimeout,
				TLSHandshakeTimeout:   defaultTLSHandshakeTimeout,
				ResponseHeaderTimeout: defaultResponseHeaderTimeout,
				MaxIdleConnections:    defaultMaxIdleConns,
				Clouddrivers: []clouddriverConfig{
					{URL: "abcd"},
					{URL: "wxyz"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := loadConfiguration(tt.input)
			require.NoError(t, err)
			require.NotNil(t, actual)
			assert.Equal(t, tt.wantOut, actual)
		})
	}
}
