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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

func (s *srv) redirect() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		dialer := net.Dialer{Timeout: time.Duration(conf.DialTimeout) * time.Second}
		client := &http.Client{
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

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		reqBody, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			log.Printf("%v", err)
			return
		}
		req.Body.Close()
		reqBodyReader := bytes.NewReader(reqBody)

		target := combineURL(s.destinationURL, req.RequestURI)
		httpRequest, err := http.NewRequestWithContext(ctx, req.Method, target, reqBodyReader)
		for k, vv := range req.Header {
			for _, v := range vv {
				httpRequest.Header.Add(k, v)
			}
		}

		resp, err := client.Do(httpRequest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			log.Printf("%v", err)
			return
		}

		defer resp.Body.Close()
		copyHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			log.Printf("%v", err)
			return
		}

		t := tracer{
			Method: req.Method,
			Request: tracerHTTP{
				Body:    base64.StdEncoding.EncodeToString(reqBody),
				Headers: req.Header,
				URI:     req.RequestURI,
			},
			Response: tracerHTTP{
				Body:       base64.StdEncoding.EncodeToString(respBody),
				Headers:    resp.Header,
				StatusCode: resp.StatusCode,
				URI:        target,
			},
		}
		json, _ := json.Marshal(t)

		log.Printf("%s", json)
		w.Write(respBody)
	}
}