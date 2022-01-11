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
	"crypto/tls"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

type srv struct {
	listenPort     uint16
	destinationURL string
	Insecure       bool
}

func (*srv) headers() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		for name, headers := range req.Header {
			for _, h := range headers {
				fmt.Fprintf(w, "%v: %v\n", name, h)
			}
		}
	}
}

type tracerHTTP struct {
	URI        string              `json:"uri,omitempty"`
	Headers    map[string][]string `json:"headers,omitempty"`
	Body       string              `json:"body,omitempty"`
	StatusCode int                 `json:"status_code,omitempty"`
}

type tracer struct {
	Method   string     `json:"method,omitempty"`
	Request  tracerHTTP `json:"request,omitempty"`
	Response tracerHTTP `json:"response,omitempty"`
}

func (s *srv) routes(mux *mux.Router) {
	mux.HandleFunc("/credentials", s.credentials())
	mux.HandleFunc("/credentials/{id}", s.credentialsByID())
	mux.HandleFunc("/_headers", s.headers())
	mux.HandleFunc("/", s.redirect())
}

func runHTTPServer(conf *configuration) {
	s := &srv{
		listenPort:     conf.ListenPort,
		destinationURL: "http://spin-clouddriver:7002",
	}
	mux := mux.NewRouter()
	s.routes(mux)
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.listenPort),
		Handler: mux,
		// Disable HTTP/2.
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}
	log.Fatal(srv.ListenAndServe())
}
