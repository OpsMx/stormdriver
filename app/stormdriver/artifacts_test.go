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

import "testing"

func Test_getArtifactAccountName(t *testing.T) {
	type args struct {
		data string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{"bad json", args{`[}`}, "", true},
		{"missing account field", args{`{"foo":"bar"}`}, "", false},
		{"has artifactAccount", args{`{"metadata":{"id":"bob"},"artifactAccount":"alice"}`}, "alice", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getArtifactAccountName([]byte(tt.args.data))
			if (err != nil) != tt.wantErr {
				t.Errorf("getArtifactAccountName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getArtifactAccountName() = %v, want %v", got, tt.want)
			}
		})
	}
}
