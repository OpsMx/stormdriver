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
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type fetchResult struct {
	err error
}

type listFetchResult struct {
	result fetchResult
	data   []interface{}
}

type featureFlag struct {
	Name    string `json:"name,omitempty"`
	Enabled bool   `json:"enabled,omitempty"`
}

type featureFetchResult struct {
	result fetchResult
	data   []featureFlag
}

type mapFetchResult struct {
	result fetchResult
	data   map[string]interface{}
}

type singletonFetchResult struct {
	result     fetchResult
	data       []byte
	statusCode int
}

func fetchListFromOneEndpoint(ctx context.Context, c chan listFetchResult, url string, headers http.Header) {
	ctx, span := tracer.Start(ctx, "fetchListFromOneEndpoint")
	defer span.End()
	span.SetAttributes(attribute.String("url", url))

	bytes, statusCode, _, err := fetchGet(ctx, url, headers)

	if err != nil {
		ret := listFetchResult{result: fetchResult{err: err}}
		c <- ret
		span.SetStatus(codes.Error, err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		c <- listFetchResult{fetchResult{nil}, []interface{}{}}
		span.SetStatus(codes.Ok, "http-404-ignored")
		return
	}

	if !statusCodeOK(statusCode) {
		msg := fmt.Errorf("%s statusCode %d", url, statusCode)
		ret := listFetchResult{result: fetchResult{err: msg}}
		c <- ret
		span.SetStatus(codes.Ok, msg.Error())
		return
	}

	var data []interface{}
	err = json.Unmarshal(bytes, &data)
	if err != nil {
		msg := fmt.Errorf("%s returned junk: %v, %s", url, err, string(bytes))
		ret := listFetchResult{result: fetchResult{err: msg}}
		c <- ret
		span.SetStatus(codes.Error, msg.Error())
		return
	}

	msg := fmt.Sprintf("received %d items", len(data))
	span.SetStatus(codes.Ok, msg)
	c <- listFetchResult{
		result: fetchResult{err: nil},
		data:   data,
	}
}

func fetchSingletonFromOneEndpoint(ctx context.Context, c chan singletonFetchResult, url string, headers http.Header) {
	_, span := tracer.Start(ctx, "fetchSingletonFromOneEndpoint")
	defer span.End()
	span.SetAttributes(attribute.String("url", url))

	bytes, statusCode, _, err := fetchGet(ctx, url, headers)

	if err != nil {
		ret := singletonFetchResult{result: fetchResult{err: err}}
		c <- ret
		span.SetStatus(codes.Error, err.Error())
		return
	}

	// handle 404 Not Found as not quite an error.
	if statusCode == http.StatusNotFound {
		ret := singletonFetchResult{statusCode: statusCode}
		c <- ret
		span.SetStatus(codes.Ok, "http-404-ignored")
		return
	}

	if !statusCodeOK(statusCode) {
		msg := fmt.Errorf("%s statusCode %d", url, statusCode)
		ret := singletonFetchResult{result: fetchResult{err: msg}}
		c <- ret
		span.SetStatus(codes.Error, msg.Error())
		return
	}

	c <- singletonFetchResult{
		result:     fetchResult{err: nil},
		data:       bytes,
		statusCode: statusCode,
	}
}

func getKeyValue(item interface{}, target string) string {
	m, ok := item.(map[string]interface{})
	if !ok {
		return ""
	}
	if v, ok := m[target].(string); ok {
		return v
	}
	return ""
}

func combineUniqueLists(c chan listFetchResult, count int, key string) []interface{} {
	ret := []interface{}{}
	seen := map[string]bool{}

	for i := 0; i < count; i++ {
		j := <-c
		if j.result.err != nil {
			log.Printf("%v", j.result.err)
			continue
		}
		if key == "" {
			ret = append(ret, j.data...)
			continue
		}

		for _, item := range j.data {
			itemKey := getKeyValue(item, key)
			if itemKey != "" && !seen[itemKey] {
				seen[itemKey] = true
				ret = append(ret, item)
			}
		}
	}
	return ret
}

func combineFeatureLists(c chan featureFetchResult, count int) []featureFlag {
	flags := map[string]bool{}
	for i := 0; i < count; i++ {
		j := <-c
		if j.result.err != nil {
			log.Printf("%v", j.result.err)
		} else {
			for _, flag := range j.data {
				flags[flag.Name] = flags[flag.Name] || flag.Enabled
			}
		}
	}

	ret := make([]featureFlag, 0, len(flags))
	for name, value := range flags {
		ret = append(ret, featureFlag{name, value})
	}
	return ret
}

func combineMaps(c chan mapFetchResult, count int) map[string]interface{} {
	ret := make(map[string]interface{})
	for i := 0; i < count; i++ {
		j := <-c
		if j.result.err != nil {
			log.Printf("%v", j.result.err)
		} else {
			for k, v := range j.data {
				ret[k] = v
			}
		}
	}
	return ret
}

func fetchGet(ctx context.Context, url string, headers http.Header) ([]byte, int, http.Header, error) {
	ctx, span := tracer.Start(ctx, "fetchGet")
	defer span.End()

	client := newHTTPClient()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	httpRequest, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	copyHeaders(httpRequest.Header, headers)
	httpRequest.Header.Set("Accept", "application/json")

	resp, err := client.Do(httpRequest)
	if err != nil {
		log.Printf("%v", err)
		span.SetStatus(codes.Error, err.Error())
		return []byte{}, -1, http.Header{}, err
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("%v", err)
		span.SetStatus(codes.Error, err.Error())
		return []byte{}, -2, http.Header{}, err
	}

	span.SetStatus(codes.Ok, "")
	return respBody, resp.StatusCode, resp.Header, nil
}

func fetchWithBody(ctx context.Context, method string, url string, headers http.Header, body []byte) ([]byte, int, http.Header, error) {
	ctx, span := tracer.Start(ctx, "fetchWithBody")
	defer span.End()

	client := newHTTPClient()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	httpRequest, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	copyHeaders(httpRequest.Header, headers)
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Content-Type", "application/json; charset=UTF-8")

	resp, err := client.Do(httpRequest)
	if err != nil {
		log.Printf("%v", err)
		span.SetStatus(codes.Error, err.Error())
		return []byte{}, -1, http.Header{}, err
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("%v", err)
		span.SetStatus(codes.Error, err.Error())
		return []byte{}, -2, http.Header{}, err
	}

	span.SetStatus(codes.Ok, "")
	return respBody, resp.StatusCode, resp.Header, nil
}

func (*srv) fetchList(key string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("content-type", "application/json")

		ctx, span := tracer.Start(req.Context(), "fetchList")
		defer span.End()

		retchan := make(chan listFetchResult)
		cds := getHealthyClouddriverURLs()

		for _, url := range cds {
			go fetchListFromOneEndpoint(ctx, retchan, combineURL(url, req.RequestURI), req.Header)
		}

		ret := combineUniqueLists(retchan, len(cds), key)

		outjson, err := json.Marshal(ret)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			span.SetStatus(codes.Error, err.Error())
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write(outjson)
			span.SetStatus(codes.Ok, "")
		}
	}
}

func (s *srv) singleItemByOptionalQueryID(v string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx, span := tracer.Start(req.Context(), "singleItemByOptionalQueryID")
		defer span.End()

		accountName := req.FormValue(v)
		if accountName == "" {
			s.fetchList("")(w, req)
			return
		}

		url, found := findCloudRoute(accountName)
		if !found {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		target := combineURL(url, req.RequestURI)
		fetchFrom(ctx, target, w, req)
	}
}

func (s *srv) singleArtifactItemByIDPath(v string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx, span := tracer.Start(req.Context(), "singleArtifactItemByIDPath")
		defer span.End()

		accountName := mux.Vars(req)[v]
		url, found := findArtifactRoute(accountName)
		if !found {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		target := combineURL(url, req.RequestURI)
		fetchFrom(ctx, target, w, req)
	}
}

func (s *srv) singleItemByIDPath(v string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx, span := tracer.Start(req.Context(), "singleItemByIDPath")
		defer span.End()

		accountName := mux.Vars(req)[v]
		url, found := findCloudRoute(accountName)
		if !found {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		target := combineURL(url, req.RequestURI)
		fetchFrom(ctx, target, w, req)
	}
}

func fetchFrom(ctx context.Context, target string, w http.ResponseWriter, req *http.Request) {
	w.Header().Set("content-type", "application/json")

	ctx, span := tracer.Start(ctx, "fetchFrom")
	defer span.End()

	data, code, headers, err := fetchGet(ctx, target, req.Header)
	if err != nil {
		log.Printf("Fetching from %s: %v", target, err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	if !statusCodeOK(code) {
		w.WriteHeader(code)
		if len(data) > 0 {
			w.Header().Set("content-type", headers.Get("content-type"))
			w.Write(data)
		}
		return
	}

	copyHeaders(w.Header(), headers)
	w.Header().Set("content-type", headers.Get("content-type"))
	w.WriteHeader(code)
	w.Write(data)
}

func getOneResponse(c chan singletonFetchResult, count int) []byte {
	ret := []byte{}

	for i := 0; i < count; i++ {
		j := <-c
		if j.result.err != nil {
			log.Printf("%v", j.result.err)
		} else if len(ret) == 0 {
			ret = j.data
		}
	}
	return ret
}

func (*srv) broadcast() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("content-type", "application/json")

		ctx, span := tracer.Start(req.Context(), "broadcast")
		defer span.End()

		retchan := make(chan singletonFetchResult)
		cds := getHealthyClouddriverURLs()

		for _, url := range cds {
			go fetchSingletonFromOneEndpoint(ctx, retchan, combineURL(url, req.RequestURI), req.Header)
		}

		ret := getOneResponse(retchan, len(cds))

		if ret == nil {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write(ret)
		}
	}
}

func (*srv) fetchMaps(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("content-type", "application/json")

	ctx, span := tracer.Start(req.Context(), "fetchMaps")
	defer span.End()

	retchan := make(chan mapFetchResult)
	cds := getHealthyClouddriverURLs()

	for _, url := range cds {
		go fetchMapFromOneEndpoint(ctx, retchan, combineURL(url, req.RequestURI), req.Header)
	}

	ret := combineMaps(retchan, len(cds))

	outjson, err := json.Marshal(ret)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusOK)
		w.Write(outjson)
	}
}

func (s *srv) fetchMapsHandler() http.HandlerFunc {
	return s.fetchMaps
}

func fetchMapFromOneEndpoint(ctx context.Context, c chan mapFetchResult, url string, headers http.Header) {
	ctx, span := tracer.Start(ctx, "fetchMapFromOneEndpoint")
	defer span.End()
	span.SetAttributes(attribute.String("url", url))

	bytes, statusCode, _, err := fetchGet(ctx, url, headers)

	if err != nil {
		ret := mapFetchResult{result: fetchResult{err: err}}
		c <- ret
		span.SetStatus(codes.Error, err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		c <- mapFetchResult{fetchResult{nil}, map[string]interface{}{}}
		span.SetStatus(codes.Ok, "http-404-ignored")
		return
	}

	if !statusCodeOK(statusCode) {
		msg := fmt.Errorf("%s statusCode %d", url, statusCode)
		ret := mapFetchResult{result: fetchResult{err: msg}}
		c <- ret
		span.SetStatus(codes.Error, msg.Error())
		return
	}

	var data map[string]interface{}
	err = json.Unmarshal(bytes, &data)
	if err != nil {
		msg := fmt.Errorf("%s returned junk: %v", url, err)
		ret := mapFetchResult{result: fetchResult{err: msg}}
		span.SetStatus(codes.Error, msg.Error())
		c <- ret
		return
	}

	msg := fmt.Sprintf("received %d items", len(data))
	span.SetStatus(codes.Ok, msg)
	c <- mapFetchResult{
		result: fetchResult{err: nil},
		data:   data,
	}
}

func fetchFeatureListFromOneEndpoint(ctx context.Context, c chan featureFetchResult, url string, headers http.Header) {
	ctx, span := tracer.Start(ctx, "fetchFeatureListFromOneEndpoint")
	defer span.End()

	bytes, statusCode, _, err := fetchGet(ctx, url, headers)

	if err != nil {
		ret := featureFetchResult{result: fetchResult{err: err}}
		c <- ret
		return
	}

	if !statusCodeOK(statusCode) {
		ret := featureFetchResult{result: fetchResult{err: fmt.Errorf("%s statusCode %d", url, statusCode)}}
		c <- ret
		return
	}

	result := featureFetchResult{result: fetchResult{err: nil}}
	err = json.Unmarshal(bytes, &result.data)
	if err != nil {
		ret := featureFetchResult{result: fetchResult{err: fmt.Errorf("%s returned junk: %v, %s", url, err, string(bytes))}}
		c <- ret
		return
	}

	c <- result
}

func (*srv) fetchFeatureList(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("content-type", "application/json")

	ctx, span := tracer.Start(req.Context(), "fetchFeatureList")
	defer span.End()

	retchan := make(chan featureFetchResult)
	cds := getHealthyClouddriverURLs()

	for _, url := range cds {
		go fetchFeatureListFromOneEndpoint(ctx, retchan, combineURL(url, req.RequestURI), req.Header)
	}

	ret := combineFeatureLists(retchan, len(cds))

	outjson, err := json.Marshal(ret)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusOK)
		w.Write(outjson)
	}
}
