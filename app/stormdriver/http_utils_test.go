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
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_copyHeaders(t *testing.T) {
	var tests = []struct {
		src        http.Header
		want       http.Header // duplicated to ensure we can also test failure cases
		expectPass bool
	}{
		{
			http.Header{},
			http.Header{},
			true,
		},
		{
			http.Header{
				"X-Header": []string{"this", "that"},
			},
			http.Header{
				"X-Header": []string{"this", "that"},
			},
			true,
		},
		{
			http.Header{
				"X-Header": []string{"this", "that"},
			},
			http.Header{
				"X-Header": []string{"this"},
			},
			false,
		},
	}

	for idx, tt := range tests {
		testname := fmt.Sprintf("%d", idx)
		t.Run(testname, func(t *testing.T) {
			ans := http.Header{}
			copyHeaders(ans, tt.src)
			if tt.expectPass {
				assert.Equal(t, tt.want, ans)
			} else {
				assert.NotEqual(t, tt.want, ans)
			}
		})
	}
}

func Test_combineURL(t *testing.T) {
	var tests = []struct {
		a, b string
		want string
	}{
		{"http://www.flame.org", "/get", "http://www.flame.org/get"},
		{"http://www.flame.org", "/", "http://www.flame.org/"},
		{"http://www.flame.org", "", "http://www.flame.org/"},
		{"http://www.flame.org/", "/get", "http://www.flame.org/get"},
		{"http://www.flame.org/", "", "http://www.flame.org/"},
	}

	for _, tt := range tests {
		testname := fmt.Sprintf("%s,%s", tt.a, tt.b)
		t.Run(testname, func(t *testing.T) {
			ans := combineURL(tt.a, tt.b)
			if ans != tt.want {
				t.Errorf("got %s, want %s", ans, tt.want)
			}
		})
	}
}
