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
	"os"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/skandragon/gohealthcheck/health"
)

type srv struct {
	listenPort uint16
	Insecure   bool
}

func (*srv) accountRoutesRequest() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("content-type", "application/json")
		ret := struct {
			Accounts         map[string]string `json:"accounts,omitempty"`
			ArtifactAccounts map[string]string `json:"artifactAccounts,omitempty"`
		}{getCloudAccountRoutes(), getArtifactAccountRoutes()}
		json, err := json.Marshal(ret)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(json)
	}
}

func (*srv) accountsRequest() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("content-type", "application/json")
		ret := struct {
			Accounts         []trackedSpinnakerAccount `json:"accounts,omitempty"`
			ArtifactAccounts []trackedSpinnakerAccount `json:"artifactAccounts,omitempty"`
		}{getCloudAccounts(), getArtifactAccounts()}
		json, err := json.Marshal(ret)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(json)
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

func loggingMiddleware(next http.Handler) http.Handler {
	return handlers.LoggingHandler(os.Stdout, next)
}

func (s *srv) routes(mux *mux.Router) {
	mux.Use(loggingMiddleware)

	mux.HandleFunc("/applications", s.fetchList).Methods(http.MethodGet)
	mux.HandleFunc("/applications/{name}/clusters", s.fetchMapsHandler()).Methods(http.MethodGet)
	mux.HandleFunc("/applications/{name}/loadBalancers", s.fetchList).Methods(http.MethodGet)
	mux.HandleFunc("/applications/{name}/serverGroupManagers", s.fetchList).Methods(http.MethodGet)
	mux.HandleFunc("/applications/{name}/serverGroups", s.fetchList).Methods(http.MethodGet)
	mux.HandleFunc("/artifacts/credentials", s.fetchUniqueList("name")).Methods(http.MethodGet)
	mux.HandleFunc("/artifacts/fetch", s.artifactsPut).Methods(http.MethodPut)
	mux.HandleFunc("/artifacts/fetch/", s.artifactsPut).Methods(http.MethodPut) // lame!
	mux.HandleFunc("/aws/images/find", s.fetchList).Methods(http.MethodGet)
	mux.HandleFunc("/aws/ops", s.cloudOpsPost()).Methods(http.MethodPost)
	mux.PathPrefix("/cache").HandlerFunc(handleCachePost).Methods("POST")
	mux.HandleFunc("/credentials", s.fetchList).Methods(http.MethodGet)
	mux.HandleFunc("/credentials/{account}", s.singleItemByIDPath("account")).Methods(http.MethodGet)
	mux.HandleFunc("/dockerRegistry/images/find", s.singleItemByOptionalQueryID("account")).Methods(http.MethodGet)
	mux.HandleFunc("/features/stages", s.fetchFeatureList).Methods(http.MethodGet)
	mux.HandleFunc("/instanceTypes", s.fetchList).Methods(http.MethodGet)
	mux.HandleFunc("/keyPairs", s.fetchList).Methods(http.MethodGet)
	mux.HandleFunc("/kubernetes/ops", s.cloudOpsPost()).Methods(http.MethodPost)
	mux.HandleFunc("/securityGroups", s.fetchMapsHandler()).Methods(http.MethodGet)
	mux.HandleFunc("/subnets/aws", s.fetchList).Methods(http.MethodGet)
	mux.PathPrefix("/applications/{name}/clusters/{account}").HandlerFunc(s.singleItemByIDPath("account")).Methods(http.MethodGet)
	mux.PathPrefix("/applications/{name}/loadBalancers/{account}").HandlerFunc(s.singleItemByIDPath("account")).Methods(http.MethodGet)
	mux.PathPrefix("/applications/{name}/serverGroups/{account}").HandlerFunc(s.singleItemByIDPath("account")).Methods(http.MethodGet)
	mux.PathPrefix("/instances/{account}").HandlerFunc(s.singleItemByIDPath("account")).Methods(http.MethodGet)
	mux.PathPrefix("/manifests/{account}").HandlerFunc(s.singleItemByIDPath("account")).Methods(http.MethodGet)
	mux.HandleFunc("/networks/aws", s.fetchList).Methods(http.MethodGet)
	mux.PathPrefix("/securityGroups/{account}").HandlerFunc(s.singleItemByIDPath("account")).Methods(http.MethodGet)
	mux.PathPrefix("/serverGroups/{account}").HandlerFunc(s.singleItemByIDPath("account")).Methods(http.MethodGet)
	mux.PathPrefix("/task").HandlerFunc(s.broadcast()).Methods(http.MethodGet)

	// internal handlers
	mux.HandleFunc("/_internal/accountRoutes", s.accountRoutesRequest()).Methods(http.MethodGet)
	mux.HandleFunc("/_internal/accounts", s.accountsRequest()).Methods(http.MethodGet)

	// Catch-all for all other actions.  These endpoints will need to be added...
	mux.PathPrefix("/").HandlerFunc(s.redirect()).Methods(http.MethodGet)
	mux.PathPrefix("/").HandlerFunc(s.failAndLog()).Methods(http.MethodPost, http.MethodConnect, http.MethodDelete, http.MethodOptions, http.MethodPatch, http.MethodPut, http.MethodTrace)
}

func runHTTPServer(conf *configuration, healthchecker *health.Health) {
	s := &srv{
		listenPort: conf.HTTPListenPort,
	}

	m := mux.NewRouter()
	// added first because order matters.
	m.HandleFunc("/health", healthchecker.HTTPHandler()).Methods(http.MethodGet)
	s.routes(m)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.listenPort),
		Handler: m,
		// Disable HTTP/2.
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}
	log.Fatal(srv.ListenAndServe())
}
