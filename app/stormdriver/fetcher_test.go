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
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_combineLists(t *testing.T) {
	var t123 []interface{}
	t123 = append(t123, 1, 2, 3)

	var t456 []interface{}
	t456 = append(t456, 4, 5, 6)

	var t789 []interface{}
	t789 = append(t789, 7, 8, 9)

	var t123456 []interface{}
	t123456 = append(t123456, 1, 2, 3, 4, 5, 6)

	var t123456789 []interface{}
	t123456789 = append(t123456789, 1, 2, 3, 4, 5, 6, 7, 8, 9)

	var tests = []struct {
		name  string
		count int
		want  []interface{}
	}{
		{
			"combine with one list",
			1,
			t123,
		},
		{
			"combine with two lists",
			2,
			t123456,
		},
		{
			"combine with three lists",
			3,
			t123456789,
		},
	}

	for _, tt := range tests {
		testname := fmt.Sprintf("%s", tt.name)
		t.Run(testname, func(t *testing.T) {
			c := make(chan listFetchResult, 100)
			for i := 0; i < tt.count; i++ {
				if i == 0 {
					c <- listFetchResult{data: t123}
				}
				if i == 1 {
					c <- listFetchResult{data: t456}
				}
				if i == 2 {
					c <- listFetchResult{data: t789}
				}
			}
			ret := combineLists(c, tt.count)
			assert.Equal(t, tt.want, ret)
		})
	}
}

func Test_getOneResponse(t *testing.T) {
	var tests = []struct {
		name string
		list []singletonFetchResult
		want []byte
	}{
		{
			"one response",
			[]singletonFetchResult{
				{
					data: []byte("this"),
				},
			},
			[]byte("this"),
		},
		{
			"two responses, 2nd with error",
			[]singletonFetchResult{
				{
					data: []byte("this"),
				},
				{
					data:   []byte("that"),
					result: fetchResult{err: fmt.Errorf("foo")},
				},
			},
			[]byte("this"),
		},
		{
			"two responses, 1st with error",
			[]singletonFetchResult{
				{
					data:   []byte("that"),
					result: fetchResult{err: fmt.Errorf("foo")},
				},
				{
					data: []byte("this"),
				},
			},
			[]byte("this"),
		},
		{
			"no valid responses returns nil",
			[]singletonFetchResult{
				{
					data:   []byte("this"),
					result: fetchResult{err: fmt.Errorf("foo")},
				},
				{
					data:   []byte("that"),
					result: fetchResult{err: fmt.Errorf("foo")},
				},
			},
			nil,
		},
	}

	for _, tt := range tests {
		testname := fmt.Sprintf("%s", tt.name)
		t.Run(testname, func(t *testing.T) {
			c := make(chan singletonFetchResult, 100)
			for i := 0; i < len(tt.list); i++ {
				c <- tt.list[i]
			}
			ret := getOneResponse(c, len(tt.list))
			assert.Equal(t, tt.want, ret)
		})
	}
}

func Test_combineMaps(t *testing.T) {
	var tests = []struct {
		name string
		list []mapFetchResult
		want map[string]interface{}
	}{
		{
			"one response",
			[]mapFetchResult{
				{
					data: map[string]interface{}{"this": 1},
				},
			},
			map[string]interface{}{"this": 1},
		},
		{
			"two responses",
			[]mapFetchResult{
				{
					data: map[string]interface{}{"this": 1},
				},
				{
					data: map[string]interface{}{"that": 2},
				},
			},
			map[string]interface{}{"this": 1, "that": 2},
		},
		{
			"two responses, 2nd with error",
			[]mapFetchResult{
				{
					data: map[string]interface{}{"this": 1},
				},
				{
					data:   map[string]interface{}{"that": 2},
					result: fetchResult{err: fmt.Errorf("foo")},
				},
			},
			map[string]interface{}{"this": 1},
		},
		{
			"two responses, 1st with error",
			[]mapFetchResult{
				{
					data:   map[string]interface{}{"that": 2},
					result: fetchResult{err: fmt.Errorf("foo")},
				},
				{
					data: map[string]interface{}{"this": 1},
				},
			},
			map[string]interface{}{"this": 1},
		},
		{
			"no valid responses returns empty map",
			[]mapFetchResult{
				{
					data:   map[string]interface{}{"this": 1},
					result: fetchResult{err: fmt.Errorf("foo")},
				},
				{
					data:   map[string]interface{}{"that": 2},
					result: fetchResult{err: fmt.Errorf("foo")},
				},
			},
			map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		testname := fmt.Sprintf("%s", tt.name)
		t.Run(testname, func(t *testing.T) {
			c := make(chan mapFetchResult, 100)
			for i := 0; i < len(tt.list); i++ {
				c <- tt.list[i]
			}
			ret := combineMaps(c, len(tt.list))
			assert.Equal(t, tt.want, ret)
		})
	}
}

func Test_combineFeatureLists(t *testing.T) {
	var tests = []struct {
		name string
		list []featureFetchResult
		want []featureFlag
	}{
		{
			"one response",
			[]featureFetchResult{
				{
					data: []featureFlag{{"this", true}},
				},
			},
			[]featureFlag{{"this", true}},
		},
		{
			"two responses",
			[]featureFetchResult{
				{
					data: []featureFlag{{"this", true}},
				},
				{
					data: []featureFlag{{"that", true}},
				},
			},
			[]featureFlag{{"this", true}, {"that", true}},
		},
		{
			"two responses, 2nd with error",
			[]featureFetchResult{
				{
					data: []featureFlag{{"this", true}},
				},
				{
					data:   []featureFlag{{"that", true}},
					result: fetchResult{err: fmt.Errorf("foo")},
				},
			},
			[]featureFlag{{"this", true}},
		},
		{
			"two responses, 1st with error",
			[]featureFetchResult{
				{
					data:   []featureFlag{{"that", true}},
					result: fetchResult{err: fmt.Errorf("foo")},
				},
				{
					data: []featureFlag{{"this", true}},
				},
			},
			[]featureFlag{{"this", true}},
		},
		{
			"no valid responses returns empty map",
			[]featureFetchResult{
				{
					data:   []featureFlag{{"this", true}},
					result: fetchResult{err: fmt.Errorf("foo")},
				},
				{
					data:   []featureFlag{{"that", true}},
					result: fetchResult{err: fmt.Errorf("foo")},
				},
			},
			[]featureFlag{},
		},
		{
			"true overrides false",
			[]featureFetchResult{
				{
					data: []featureFlag{{"this", false}},
				},
				{
					data: []featureFlag{{"this", true}},
				},
			},
			[]featureFlag{{"this", true}},
		},
		{
			"false doesn't override true",
			[]featureFetchResult{
				{
					data: []featureFlag{{"this", true}},
				},
				{
					data: []featureFlag{{"this", false}},
				},
			},
			[]featureFlag{{"this", true}},
		},
	}

	for _, tt := range tests {
		testname := fmt.Sprintf("%s", tt.name)
		t.Run(testname, func(t *testing.T) {
			c := make(chan featureFetchResult, 100)
			for i := 0; i < len(tt.list); i++ {
				c <- tt.list[i]
			}
			ret := combineFeatureLists(c, len(tt.list))
			assert.ElementsMatch(t, tt.want, ret)
		})
	}
}

func Test_getKeyValue(t *testing.T) {
	type args struct {
		json   string
		target string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"not a map", args{`[]`, "name"}, ""},
		{"has key type of string", args{`{"name": "alice"}`, "name"}, "alice"},
		{"returns empty for not-string value", args{`{"name": []}`, "name"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var item interface{}
			err := json.Unmarshal([]byte(tt.args.json), &item)
			if err != nil {
				t.Fatal(err)
			}
			if got := getKeyValue(item, tt.args.target); got != tt.want {
				t.Errorf("getKeyValue() = %v, want %v", got, tt.want)
			}
		})
	}
}
