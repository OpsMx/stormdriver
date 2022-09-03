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
	"log"
	"net/http"

	"github.com/OpsMx/go-app-base/httputil"
)

func handleCachePost(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("content-type", "application/json")

	data, err := io.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		log.Printf("Unable to read body in handleCachePost: %v", err)
		return
	}

	var item AccountStruct
	err = json.Unmarshal(data, &item)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		log.Printf("Unable to parse body in handleCachePost: %v", err)
		return
	}

	request64 := base64.StdEncoding.EncodeToString(data)
	log.Printf("Request %s", request64)
	log.Printf("Request headers: %#v", req.Header)

	accountName := item.AccountName()
	if accountName == "" {
		log.Printf("No account or credentials in request")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	url, found := clouddriverManager.findCloudRoute(accountName)
	if !found {
		log.Printf("Warning: account %s has no route", accountName)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	target := combineURL(url.URL, req.RequestURI)
	responseBody, code, _, err := fetchWithBody(req.Context(), req.Method, target, url.token, req.Header, data)
	response64 := base64.StdEncoding.EncodeToString(responseBody)
	log.Printf("Response: code=%d %s", code, response64)

	if err != nil {
		log.Printf("Post error to %s: %v", target, err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if !httputil.StatusCodeOK(code) {
		w.WriteHeader(code)
		return
	}
	w.WriteHeader(http.StatusOK)
	httputil.CheckedWrite(w, responseBody)
}
