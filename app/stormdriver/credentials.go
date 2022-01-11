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
	"net"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

type fetchResult struct {
	err error
}

type credentialsFetchResult struct {
	result fetchResult
	data   []interface{}
}

func newHTTPClient() *http.Client {
	dialer := net.Dialer{Timeout: time.Duration(conf.DialTimeout) * time.Second}
	return &http.Client{
		Timeout: time.Duration(conf.ClientTimeout) * time.Second,
		Transport: &http.Transport{
			Dial:                  dialer.Dial,
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   time.Duration(conf.TLSHandshakeTimeout) * time.Second,
			ResponseHeaderTimeout: time.Duration(conf.ResponseHeaderTimeout) * time.Second,
			ExpectContinueTimeout: time.Second,
			MaxIdleConns:          conf.MaxIdleConnections,
			DisableCompression:    true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
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

func fetchCredentials(c chan credentialsFetchResult, url string, headers http.Header) {
	bytes, statusCode, err := fetchGet(url, headers)

	if err != nil {
		ret := credentialsFetchResult{result: fetchResult{err: err}}
		c <- ret
		return
	}

	if !statusCodeOK(statusCode) {
		ret := credentialsFetchResult{result: fetchResult{err: fmt.Errorf("%s statusCode %d", url, statusCode)}}
		c <- ret
		return
	}

	var data []interface{}
	err = json.Unmarshal(bytes, &data)
	if err != nil {
		ret := credentialsFetchResult{result: fetchResult{err: fmt.Errorf("%s returned junk: %v, %s", url, err, string(bytes))}}
		c <- ret
		return
	}

	c <- credentialsFetchResult{
		result: fetchResult{err: nil},
		data:   data,
	}
}

func combineCredentials(c chan credentialsFetchResult, count int) []interface{} {
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

func (s *srv) credentials() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		retchan := make(chan credentialsFetchResult)
		cds := getClouddriverURLs()

		for _, url := range cds {
			go fetchCredentials(retchan, combineURL(url, req.RequestURI), req.Header)
		}

		ret := combineCredentials(retchan, len(cds))

		outjson, err := json.Marshal(ret)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write(outjson)
		}
	}
}

func (s *srv) credentialsByID() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		vars := mux.Vars(req)

		url, found := findAccountRoute(vars["id"])
		if !found {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		target := combineURL(url, req.RequestURI)

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
}
