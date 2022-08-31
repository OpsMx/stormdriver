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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/OpsMx/go-app-base/httputil"
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
			Accounts         map[string]URLAndPriority `json:"accounts,omitempty"`
			ArtifactAccounts map[string]URLAndPriority `json:"artifactAccounts,omitempty"`
		}{getCloudAccountRoutes(), getArtifactAccountRoutes()}
		json, err := json.Marshal(ret)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		httputil.CheckedWrite(w, json)
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
		httputil.CheckedWrite(w, json)
	}
}

type tracerHTTP struct {
	URI        string              `json:"uri,omitempty"`
	Headers    map[string][]string `json:"headers,omitempty"`
	Body       string              `json:"body,omitempty"`
	StatusCode int                 `json:"status_code,omitempty"`
}

type tracerContents struct {
	Method   string     `json:"method,omitempty"`
	Request  tracerHTTP `json:"request,omitempty"`
	Response tracerHTTP `json:"response,omitempty"`
}

func loggingMiddleware(next http.Handler) http.Handler {
	return handlers.LoggingHandler(os.Stdout, next)
}

func (s *srv) routes(r *mux.Router) {
	r.HandleFunc("/applications", s.fetchList("")).Methods(http.MethodGet)
	r.HandleFunc("/applications/{name}/clusters", s.fetchMapsHandler()).Methods(http.MethodGet)
	r.HandleFunc("/applications/{name}/loadBalancers", s.fetchList("")).Methods(http.MethodGet)
	r.HandleFunc("/applications/{name}/serverGroupManagers", s.fetchList("")).Methods(http.MethodGet)
	r.HandleFunc("/applications/{name}/serverGroups", s.fetchList("")).Methods(http.MethodGet)
	r.HandleFunc("/artifacts/credentials", s.fetchList("name")).Methods(http.MethodGet)
	r.HandleFunc("/artifacts/fetch", s.artifactsPut).Methods(http.MethodPut)
	r.HandleFunc("/artifacts/fetch/", s.artifactsPut).Methods(http.MethodPut) // lame!
	r.HandleFunc("/artifacts/account/{account}/names", s.singleArtifactItemByIDPath("account")).Methods(http.MethodGet)
	r.HandleFunc("/artifacts/account/{account}/versions", s.singleArtifactItemByIDPath("account")).Methods(http.MethodGet)
	r.HandleFunc("/aws/images/find", s.fetchList("")).Methods(http.MethodGet)
	r.HandleFunc("/aws/ops", s.cloudOpsPost()).Methods(http.MethodPost)
	r.PathPrefix("/cache").HandlerFunc(handleCachePost).Methods("POST")
	r.HandleFunc("/credentials", s.fetchList("name")).Methods(http.MethodGet)
	r.HandleFunc("/credentials/{account}", s.singleItemByIDPath("account")).Methods(http.MethodGet)
	r.HandleFunc("/dockerRegistry/images/find", s.singleItemByOptionalQueryID("account")).Methods(http.MethodGet)
	r.HandleFunc("/features/stages", s.fetchFeatureList).Methods(http.MethodGet)
	r.HandleFunc("/instanceTypes", s.fetchList("")).Methods(http.MethodGet)
	r.HandleFunc("/keyPairs", s.fetchList("")).Methods(http.MethodGet)
	r.HandleFunc("/kubernetes/ops", s.cloudOpsPost()).Methods(http.MethodPost)
	r.HandleFunc("/securityGroups", s.fetchMapsHandler()).Methods(http.MethodGet)
	r.HandleFunc("/subnets/aws", s.fetchList("")).Methods(http.MethodGet)
	r.PathPrefix("/applications/{name}/clusters/{account}").HandlerFunc(s.singleItemByIDPath("account")).Methods(http.MethodGet)
	r.PathPrefix("/applications/{name}/loadBalancers/{account}").HandlerFunc(s.singleItemByIDPath("account")).Methods(http.MethodGet)
	r.PathPrefix("/applications/{name}/serverGroups/{account}").HandlerFunc(s.singleItemByIDPath("account")).Methods(http.MethodGet)
	r.PathPrefix("/instances/{account}").HandlerFunc(s.singleItemByIDPath("account")).Methods(http.MethodGet)
	r.PathPrefix("/manifests/{account}").HandlerFunc(s.singleItemByIDPath("account")).Methods(http.MethodGet)
	r.HandleFunc("/networks/aws", s.fetchList("")).Methods(http.MethodGet)
	r.PathPrefix("/securityGroups/{account}").HandlerFunc(s.singleItemByIDPath("account")).Methods(http.MethodGet)
	r.PathPrefix("/serverGroups/{account}").HandlerFunc(s.singleItemByIDPath("account")).Methods(http.MethodGet)
	r.PathPrefix("/task").HandlerFunc(s.broadcast()).Methods(http.MethodGet)

	// internal handlers
	r.HandleFunc("/_internal/accountRoutes", s.accountRoutesRequest()).Methods(http.MethodGet)
	r.HandleFunc("/_internal/accounts", s.accountsRequest()).Methods(http.MethodGet)

	// Catch-all for all other actions.  These endpoints will need to be added...
	r.PathPrefix("/").HandlerFunc(s.redirect()).Methods(http.MethodGet)
	r.PathPrefix("/").HandlerFunc(s.failAndLog()).Methods(http.MethodPost, http.MethodConnect, http.MethodDelete, http.MethodOptions, http.MethodPatch, http.MethodPut, http.MethodTrace)
}

func runHTTPServer(ctx context.Context, conf *configuration, healthchecker *health.Health) {
	s := &srv{
		listenPort: conf.HTTPListenPort,
	}

	r := mux.NewRouter()
	// added first because order matters.
	r.HandleFunc("/health", healthchecker.HTTPHandler()).Methods(http.MethodGet)
	s.routes(r)

	r.Use(loggingMiddleware)
	//r.Use(otelmux.Middleware("stormdriver-clouddriver"))

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.listenPort),
		Handler: r,
		// Disable HTTP/2.
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}
	log.Fatal(srv.ListenAndServe())
}
