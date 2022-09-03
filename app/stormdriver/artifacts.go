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

type artifactAccountFetchRequest struct {
	ArtifactAccount string `json:"artifactAccount,omitempty"`
}

func getArtifactAccountName(data []byte) (string, error) {
	var item artifactAccountFetchRequest
	err := json.Unmarshal(data, &item)
	if err != nil {
		return "", err
	}
	return item.ArtifactAccount, nil
}

func (*srv) artifactsPut(w http.ResponseWriter, req *http.Request) {
	data, err := io.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		zap.S().Errorw("io.ReadAll", "error", err)
		return
	}

	accountName, err := getArtifactAccountName(data)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		zap.S().Errorw("getArtifactAccountName", "error", err)
		return
	}

	if accountName == "" {
		zap.S().Warnw("no account name in request")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	url, found := clouddriverManager.findArtifactRoute(accountName)
	if !found {
		zap.S().Warnw("no route for artifact account", "accountName", accountName)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	target := combineURL(url.URL, req.RequestURI)
	responseBody, code, responseHeaders, err := fetchWithBody(req.Context(), req.Method, target, url.token, req.Header, data)
	if err != nil {
		zap.S().Errorw("fetchWithBody", "error", err, "target", target, "method", req.Method, "hasToken", url.token != "")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if !httputil.StatusCodeOK(code) {
		w.Header().Set("content-type", responseHeaders.Get("content-type"))
		w.WriteHeader(code)
		return
	}
	w.WriteHeader(http.StatusOK)
	httputil.CheckedWrite(w, responseBody)
}
