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

// AccountStruct is a simple parse helper which contains only a small number
// of fields, specifically "account", so we can look at that field easily.
// If account is not set, we will look at Credentials instead.
type AccountStruct struct {
	Account     string `json:"account,omitempty"`
	Credentials string `json:"credentials,omitempty"`
}

// AccountName returns the "best" name for this object's account, or "" if
// there isn't a best option.
func (a *AccountStruct) AccountName() string {
	if len(a.Account) > 0 {
		return a.Account
	}
	if len(a.Credentials) > 0 {
		return a.Credentials
	}
	return ""
}

func (*srv) cloudOpsPost() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("content-type", "application/json")

		data, err := io.ReadAll(req.Body)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			zap.S().Errorw("reading body", "error", err)
			return
		}

		var list []map[string]AccountStruct
		err = json.Unmarshal(data, &list)
		if err != nil {
			zap.S().Errorw("parse body", "error", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		foundURLs := map[string]URLAndPriority{}
		foundAccounts := map[string]bool{}

		for idx, item := range list {
			for requestType, subitem := range item {
				accountName := subitem.AccountName()
				if accountName == "" {
					zap.S().Warnw("no account or credentials found for cloud request", "index", idx, "requestType", requestType)
					continue
				}
				foundAccounts[accountName] = true
				url, found := clouddriverManager.findCloudRoute(accountName)
				if !found {
					zap.S().Warnw("no route for account", "accountName", accountName)
					continue
				}
				foundURLs[url.key()] = url
			}
		}

		foundAccountNames := keysForMap(foundAccounts)

		if len(foundURLs) == 0 {
			zap.S().Errorw("no routes found for any accounts in request", "accountNames", foundAccountNames)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		if len(foundURLs) != 1 {
			zap.S().Warnw("multiple routes found", "accountNames", foundAccountNames)
		}

		// will contain at least one element due to checking len(foundURLs) above
		foundURLNames := keysForMap(foundURLs)
		url := foundURLs[foundURLNames[0]]

		target := combineURL(url.URL, req.RequestURI)
		responseBody, code, _, err := fetchWithBody(req.Context(), req.Method, target, url.token, req.Header, data)

		if err != nil {
			zap.S().Errorw("post failed", "url", target, "error", err)
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
}
