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
	"io"
	"log"
	"net/http"

	"gopkg.in/yaml.v3"
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
			log.Printf("Unable to read body in cloudOpsPost: %v", err)
			return
		}

		var list []map[string]AccountStruct
		err = yaml.Unmarshal(data, &list)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			log.Printf("Unable to parse body in cloudOpsPost: %v", err)
			return
		}

		request64 := base64.StdEncoding.EncodeToString(data)
		log.Printf("Request %s", request64)
		log.Printf("Request headers: %#v", req.Header)

		foundURLs := map[string]bool{}
		foundAccounts := map[string]bool{}

		for idx, item := range list {
			for requestType, subitem := range item {
				accountName := subitem.AccountName()
				if accountName == "" {
					log.Printf("No account or credentials in request index %d, type %s", idx, requestType)
					continue
				}
				foundAccounts[accountName] = true
				url, found := findAccountRoute(accountName)
				if !found {
					log.Printf("Warning: account %s has no route", accountName)
					continue
				}
				foundURLs[url] = true
			}
		}

		foundAccountNames := keysForMapStringToBool(foundAccounts)

		if len(foundURLs) == 0 {
			log.Printf("Error: no routes found for any accounts in request: %v", foundAccountNames)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		if len(foundURLs) != 1 {
			log.Printf("WARNING: multiple routes found for accounts in request: %v.  Will try one at random.", foundAccountNames)
		}

		// will contain at least one element due to checking len(foundURLs) above
		foundURLNames := keysForMapStringToBool(foundURLs)

		target := combineURL(foundURLNames[0], req.RequestURI)
		responseBody, code, _, err := fetchPost(target, req.Header, data)
		response64 := base64.StdEncoding.EncodeToString(responseBody)
		log.Printf("Response: code=%d %s", code, response64)

		if err != nil {
			log.Printf("Post error to %s: %v", target, err)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if !statusCodeOK(code) {
			w.WriteHeader(code)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(responseBody)
	}
}

func keysForMapStringToBool(m map[string]bool) []string {
	ret := make([]string, 0, len(m))
	for k := range m {
		ret = append(ret, k)
	}
	return ret
}
