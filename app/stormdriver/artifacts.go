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
	"io"
	"log"
	"net/http"

	"gopkg.in/yaml.v3"
)

func (*srv) artifactsPut(w http.ResponseWriter, req *http.Request) {
	data, err := io.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		log.Printf("%s: Unable to read body: %v", trace(), err)
		return
	}

	var item struct {
		ArtifactAccount string `json:"artifactAccount,omitempty"`
	}
	err = yaml.Unmarshal(data, &item)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		log.Printf("%s: Unable to parse body: %v", trace(), err)
		return
	}

	accountName := item.ArtifactAccount
	if accountName == "" {
		log.Printf("No artifactAccount in request")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	url, found := findArtifactRoute(accountName)
	if !found {
		log.Printf("Warning: artifactAccount %s has no route", accountName)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	target := combineURL(url, req.RequestURI)
	responseBody, code, responseHeaders, err := fetchWithBody(req.Method, target, req.Header, data)

	if err != nil {
		log.Printf("PUT error to %s: %v", target, err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if !statusCodeOK(code) {
		w.Header().Set("content-type", responseHeaders.Get("content-type"))
		w.WriteHeader(code)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(responseBody)
}
