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
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_combineCredentials(t *testing.T) {
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
			c := make(chan credentialsFetchResult, 100)
			for i := 0; i < tt.count; i++ {
				if i == 0 {
					c <- credentialsFetchResult{data: t123}
				}
				if i == 1 {
					c <- credentialsFetchResult{data: t456}
				}
				if i == 2 {
					c <- credentialsFetchResult{data: t789}
				}
			}
			ret := combineCredentials(c, tt.count)
			assert.Equal(t, tt.want, ret)
		})
	}
}
