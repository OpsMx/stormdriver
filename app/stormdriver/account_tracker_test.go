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
)

func Test_mergeIfUnique(t *testing.T) {
	type args struct {
		url              URLAndPriority
		instanceAccounts []trackedSpinnakerAccount
		routes           map[string]URLAndPriority
		newAccounts      []trackedSpinnakerAccount
	}
	tests := []struct {
		name       string
		args       args
		want       []trackedSpinnakerAccount
		wantRoutes map[string]URLAndPriority
	}{
		{
			"no duplicate",
			args{
				URLAndPriority{"url2", 0},
				[]trackedSpinnakerAccount{{"a2", "aws"}},
				map[string]URLAndPriority{"a1": {"url1", 0}},
				[]trackedSpinnakerAccount{{"a1", "aws"}},
			},
			[]trackedSpinnakerAccount{
				{"a1", "aws"},
				{"a2", "aws"},
			},
			map[string]URLAndPriority{
				"a1": {"url1", 0},
				"a2": {"url2", 0},
			},
		},

		{
			"duplicate item",
			args{
				URLAndPriority{"url2", 0},
				[]trackedSpinnakerAccount{{"a2", "aws"}},
				map[string]URLAndPriority{"a2": {"url1", 0}},
				[]trackedSpinnakerAccount{{"a2", "aws"}},
			},
			[]trackedSpinnakerAccount{
				{"a2", "aws"},
			},
			map[string]URLAndPriority{
				"a2": {"url1", 0},
			},
		},

		{
			"Higher priority already exists",
			args{
				URLAndPriority{"url2", 1},
				[]trackedSpinnakerAccount{{"a2", "aws"}},
				map[string]URLAndPriority{"a2": {"url1", 0}},
				[]trackedSpinnakerAccount{{"a2", "aws"}},
			},
			[]trackedSpinnakerAccount{
				{"a2", "aws"},
			},
			map[string]URLAndPriority{
				"a2": {"url2", 1},
			},
		},

		{
			"Higher priority found",
			args{
				URLAndPriority{"url2", 0},
				[]trackedSpinnakerAccount{{"a2", "aws"}},
				map[string]URLAndPriority{"a2": {"url1", 1}},
				[]trackedSpinnakerAccount{{"a2", "aws"}},
			},
			[]trackedSpinnakerAccount{
				{"a2", "aws"},
			},
			map[string]URLAndPriority{
				"a2": {"url1", 1},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routes := tt.args.routes
			got := mergeIfUnique(tt.args.url, tt.args.instanceAccounts, routes, tt.args.newAccounts)
			assert.ElementsMatch(t, got, tt.want)
			assert.Equal(t, routes, tt.wantRoutes)
		})
	}
}
