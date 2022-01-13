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
	"net"
	"net/http"
	"time"
)

func statusCodeOK(statusCode int) bool {
	return statusCode >= 200 && statusCode <= 299
}

var ignoredHeaders = map[string]bool{
	"Accept-Encoding": true,
	"Connection":      true,
	"Content-Length":  true,
	"Content-Type":    true,
	"User-Agent":      true,
}

func copyHeaders(dst, src http.Header) {
	for k, vv := range src {
		if ignoredHeaders[k] {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func combineURL(base, uri string) string {
	if len(uri) == 0 {
		uri = "/"
	}
	if uri[0] != '/' {
		uri = "/" + uri
	}
	hasSlash := base[len(base)-1:] == "/"
	if hasSlash {
		return base[0:len(base)-1] + uri
	}
	return base + uri
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
