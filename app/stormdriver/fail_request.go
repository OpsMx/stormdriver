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
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"

	"go.uber.org/zap"
)

func (s *srv) failAndLog() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		reqBody, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			zap.S().Errorw("io.ReadAll", "error", err)
			return
		}
		req.Body.Close()

		t := tracerContents{
			Method: req.Method,
			Request: tracerHTTP{
				Body:    base64.StdEncoding.EncodeToString(reqBody),
				Headers: simplifyHeadersForLogging(req.Header),
				URI:     req.RequestURI,
			},
		}
		json, _ := json.Marshal(t)

		zap.S().Infof("%s", json)

		// return not available for all of these
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}
