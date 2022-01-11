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
	"encoding/json"
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

func (*srv) accountRoutesRequest() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		routes := getAccountRoutes()
		json, err := json.Marshal(routes)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(json)
	}
}

func (*srv) healthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
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
	mux.HandleFunc("/credentials", s.fetchList()).Methods(http.MethodGet)
	mux.HandleFunc("/applications", s.fetchList()).Methods(http.MethodGet)
	mux.HandleFunc("/credentials/{id}", s.singleItemByIDPath()).Methods(http.MethodGet)

	// internal handlers
	mux.HandleFunc("/health", s.healthHandler()).Methods(http.MethodGet)
	mux.HandleFunc("/_internal/accountRoutes", s.accountRoutesRequest()).Methods(http.MethodGet)

	// Catch-all for all other actions.  These endpoints will need to be added...
	mux.PathPrefix("/").HandlerFunc(s.redirect())
}

func runHTTPServer(conf *configuration) {
	urls := getClouddriverURLs()
	s := &srv{
		listenPort:     conf.ListenPort,
		destinationURL: urls[0],
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
