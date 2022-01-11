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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

type fetchResult struct {
	err error
}

type listFetchResult struct {
	result fetchResult
	data   []interface{}
}

func fetchFromOneEndpoint(c chan listFetchResult, url string, headers http.Header) {
	bytes, statusCode, err := fetchGet(url, headers)

	if err != nil {
		ret := listFetchResult{result: fetchResult{err: err}}
		c <- ret
		return
	}

	if !statusCodeOK(statusCode) {
		ret := listFetchResult{result: fetchResult{err: fmt.Errorf("%s statusCode %d", url, statusCode)}}
		c <- ret
		return
	}

	var data []interface{}
	err = json.Unmarshal(bytes, &data)
	if err != nil {
		ret := listFetchResult{result: fetchResult{err: fmt.Errorf("%s returned junk: %v, %s", url, err, string(bytes))}}
		c <- ret
		return
	}

	c <- listFetchResult{
		result: fetchResult{err: nil},
		data:   data,
	}
}

func combineLists(c chan listFetchResult, count int) []interface{} {
	var ret []interface{}
	for i := 0; i < count; i++ {
		j := <-c
		if j.result.err != nil {
			log.Printf("%v", j.result.err)
		} else {
			ret = append(ret, j.data...)
		}
	}
	return ret
}

func fetchGet(url string, headers http.Header) ([]byte, int, error) {
	client := newHTTPClient()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	httpRequest, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	copyHeaders(httpRequest.Header, headers)

	resp, err := client.Do(httpRequest)
	if err != nil {
		log.Printf("%v", err)
		return []byte{}, -1, err
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("%v", err)
		return []byte{}, -2, err
	}

	return respBody, resp.StatusCode, nil
}

func (s *srv) fetchList() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		retchan := make(chan listFetchResult)
		cds := getClouddriverURLs()

		for _, url := range cds {
			go fetchFromOneEndpoint(retchan, combineURL(url, req.RequestURI), req.Header)
		}

		ret := combineLists(retchan, len(cds))

		outjson, err := json.Marshal(ret)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write(outjson)
		}
	}
}

func (s *srv) singleItemByOptionalQueryID(v string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		accountName := req.FormValue(v)
		if accountName == "" {
			s.fetchList()
			return
		}

		url, found := findAccountRoute(accountName)
		if !found {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		target := combineURL(url, req.RequestURI)
		fetchFrom(target, w, req)
	}
}

func (s *srv) singleItemByIDPath(v string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		accountName := mux.Vars(req)[v]
		url, found := findAccountRoute(accountName)
		if !found {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		target := combineURL(url, req.RequestURI)
		fetchFrom(target, w, req)
	}
}

func fetchFrom(target string, w http.ResponseWriter, req *http.Request) {
	data, code, err := fetchGet(target, req.Header)
	if err != nil {
		log.Printf("Fetching from %s: %v", target, err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	if !statusCodeOK(code) {
		w.WriteHeader(code)
		if len(data) > 0 {
			w.Write(data)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
