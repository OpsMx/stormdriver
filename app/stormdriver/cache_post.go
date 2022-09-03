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
	"io"
	"net/http"

	"github.com/OpsMx/go-app-base/httputil"
	"go.uber.org/zap"
)

func handleCachePost(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("content-type", "application/json")

	data, err := io.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		zap.S().Errorw("NewRequestWithContext", "error", err)
		return
	}

	var item AccountStruct
	err = json.Unmarshal(data, &item)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		zap.S().Errorw("Unmarshal", "error", err)
		return
	}

	accountName := item.AccountName()
	if accountName == "" {
		zap.S().Warn("no account or credentials found")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	url, found := clouddriverManager.findCloudRoute(accountName)
	if !found {
		zap.S().Warnw("no route for account", "account", accountName)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	target := combineURL(url.URL, req.RequestURI)
	responseBody, code, _, err := fetchWithBody(req.Context(), req.Method, target, url.token, req.Header, data)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		zap.S().Errorw("fetchWithBody", "method", req.Method, "target", target, "hasToken", url.token != "", "error", err)
		return
	}
	if !httputil.StatusCodeOK(code) {
		w.WriteHeader(code)
		return
	}
	w.WriteHeader(http.StatusOK)
	httputil.CheckedWrite(w, responseBody)
}
