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
	"net/http"

	"github.com/OpsMx/go-app-base/httputil"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
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

func fetchListFromOneEndpoint(ctx context.Context, c chan listFetchResult, url string, token string, headers http.Header) {
	bytes, statusCode, _, err := fetchGet(ctx, url, token, headers)

	if err != nil {
		ret := listFetchResult{result: fetchResult{err: err}}
		c <- ret
		return
	}

	if statusCode == http.StatusNotFound {
		c <- listFetchResult{fetchResult{nil}, []interface{}{}}
		return
	}

	if !httputil.StatusCodeOK(statusCode) {
		msg := fmt.Errorf("%s statusCode %d", url, statusCode)
		ret := listFetchResult{result: fetchResult{err: msg}}
		c <- ret
		return
	}

	var data []interface{}
	err = json.Unmarshal(bytes, &data)
	if err != nil {
		msg := fmt.Errorf("%s returned junk: %v, %s", url, err, string(bytes))
		ret := listFetchResult{result: fetchResult{err: msg}}
		c <- ret
		return
	}

	c <- listFetchResult{
		result: fetchResult{err: nil},
		data:   data,
	}
}

func fetchSingletonFromOneEndpoint(ctx context.Context, c chan singletonFetchResult, url string, token string, headers http.Header) {
	bytes, statusCode, _, err := fetchGet(ctx, url, token, headers)

	if err != nil {
		ret := singletonFetchResult{result: fetchResult{err: err}}
		c <- ret
		return
	}

	// handle 404 Not Found as not quite an error.
	if statusCode == http.StatusNotFound {
		ret := singletonFetchResult{statusCode: statusCode}
		c <- ret
		return
	}

	if !httputil.StatusCodeOK(statusCode) {
		msg := fmt.Errorf("%s statusCode %d", url, statusCode)
		ret := singletonFetchResult{result: fetchResult{err: msg}}
		c <- ret
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
			zap.S().Errorw("failed to fetch", "error", j.result.err)
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
			zap.S().Errorw("failed to fetch", "error", j.result.err)
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
			zap.S().Errorw("failed to fetch", "error", j.result.err)
		} else {
			for k, v := range j.data {
				ret[k] = v
			}
		}
	}
	return ret
}

func fetchGet(ctx context.Context, url string, token string, headers http.Header) ([]byte, int, http.Header, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	httpRequest, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		zap.S().Errorw("http.NewRequestWithContext", "error", err)
		return []byte{}, -1, http.Header{}, err
	}

	copyHeaders(httpRequest.Header, headers)
	httpRequest.Header.Set("Accept", "application/json")
	if token != "" {
		httpRequest.Header.Set("authorization", fmt.Sprintf("Bearer %s", token))
	}
	resp, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		zap.S().Errorw("http.DefaultClient.Do", "error", err)
		return []byte{}, -1, http.Header{}, err
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		zap.S().Errorw("io.ReadAll", "error", err)
		return []byte{}, -2, http.Header{}, err
	}

	return respBody, resp.StatusCode, resp.Header, nil
}

func fetchWithBody(ctx context.Context, method string, url string, token string, headers http.Header, body []byte) ([]byte, int, http.Header, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	httpRequest, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		zap.S().Errorw("http.NewRequestWithContext", "method", method, "url", url, "hasToken", token != "", "error", err)
		return []byte{}, -1, http.Header{}, err
	}

	copyHeaders(httpRequest.Header, headers)
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Content-Type", "application/json; charset=UTF-8")
	if token != "" {
		httpRequest.Header.Set("authorization", fmt.Sprintf("Bearer %s", token))
	}

	resp, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		zap.S().Errorw("http.DefaultClient.Do", "method", method, "url", url, "hasToken", token != "", "error", err)
		return []byte{}, -1, http.Header{}, err
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		zap.S().Errorw("io.ReadAll", "method", method, "url", url, "hasToken", token != "", "error", err)
		return []byte{}, -2, http.Header{}, err
	}

	return respBody, resp.StatusCode, resp.Header, nil
}

func (*srv) fetchList(key string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("content-type", "application/json")

		retchan := make(chan listFetchResult)
		cds := clouddriverManager.getHealthyClouddriverURLs()

		for _, url := range cds {
			go fetchListFromOneEndpoint(req.Context(), retchan, combineURL(url.URL, req.RequestURI), url.token, req.Header)
		}

		ret := combineUniqueLists(retchan, len(cds), key)

		outjson, err := json.Marshal(ret)
		if err != nil {
			zap.S().Errorw("json.Marshal", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
			httputil.CheckedWrite(w, outjson)
		}
	}
}

func (s *srv) singleItemByOptionalQueryID(v string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		accountName := req.FormValue(v)
		if accountName == "" {
			s.fetchList("")(w, req)
			return
		}

		url, found := clouddriverManager.findCloudRoute(accountName)
		if !found {
			zap.S().Warnw("no route", "accountName", accountName)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		target := combineURL(url.URL, req.RequestURI)
		fetchFrom(req.Context(), target, url.token, w, req)
	}
}

func (s *srv) singleArtifactItemByIDPath(v string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		accountName := mux.Vars(req)[v]
		url, found := clouddriverManager.findArtifactRoute(accountName)
		if !found {
			zap.S().Warnw("no route for artifactAccount", "accountName", accountName)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		target := combineURL(url.URL, req.RequestURI)
		fetchFrom(req.Context(), target, url.token, w, req)
	}
}

func (s *srv) singleItemByIDPath(v string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		accountName := mux.Vars(req)[v]
		url, found := clouddriverManager.findCloudRoute(accountName)
		if !found {
			zap.S().Warnw("no route", "accountName", accountName)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		target := combineURL(url.URL, req.RequestURI)
		fetchFrom(req.Context(), target, url.token, w, req)
	}
}

func fetchFrom(ctx context.Context, target string, token string, w http.ResponseWriter, req *http.Request) {
	w.Header().Set("content-type", "application/json")

	data, code, headers, err := fetchGet(ctx, target, token, req.Header)
	if err != nil {
		zap.S().Errorw("fetchGet", "target", target, "hasToken", token != "", "error", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	if !httputil.StatusCodeOK(code) {
		w.WriteHeader(code)
		if len(data) > 0 {
			w.Header().Set("content-type", headers.Get("content-type"))
			httputil.CheckedWrite(w, data)
		}
		return
	}

	copyHeaders(w.Header(), headers)
	w.Header().Set("content-type", headers.Get("content-type"))
	w.WriteHeader(code)
	httputil.CheckedWrite(w, data)
}

func getOneResponse(c chan singletonFetchResult, count int) []byte {
	ret := []byte{}

	for i := 0; i < count; i++ {
		j := <-c
		if j.result.err != nil {
			zap.S().Warnw("failed to fetch", "error", j.result.err)
		} else if len(ret) == 0 {
			ret = j.data
		}
	}
	return ret
}

func (*srv) broadcast() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("content-type", "application/json")

		retchan := make(chan singletonFetchResult)
		cds := clouddriverManager.getHealthyClouddriverURLs()

		for _, url := range cds {
			go fetchSingletonFromOneEndpoint(req.Context(), retchan, combineURL(url.URL, req.RequestURI), url.token, req.Header)
		}

		ret := getOneResponse(retchan, len(cds))

		if ret == nil {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusOK)
			httputil.CheckedWrite(w, ret)
		}
	}
}

func (*srv) fetchMaps(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("content-type", "application/json")

	retchan := make(chan mapFetchResult)
	cds := clouddriverManager.getHealthyClouddriverURLs()

	for _, url := range cds {
		go fetchMapFromOneEndpoint(req.Context(), retchan, combineURL(url.URL, req.RequestURI), url.token, req.Header)
	}

	ret := combineMaps(retchan, len(cds))

	outjson, err := json.Marshal(ret)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusOK)
		httputil.CheckedWrite(w, outjson)
	}
}

func (s *srv) fetchMapsHandler() http.HandlerFunc {
	return s.fetchMaps
}

func fetchMapFromOneEndpoint(ctx context.Context, c chan mapFetchResult, url string, token string, headers http.Header) {
	bytes, statusCode, _, err := fetchGet(ctx, url, token, headers)

	if err != nil {
		ret := mapFetchResult{result: fetchResult{err: err}}
		c <- ret
		return
	}

	if statusCode == http.StatusNotFound {
		c <- mapFetchResult{fetchResult{nil}, map[string]interface{}{}}
		return
	}

	if !httputil.StatusCodeOK(statusCode) {
		msg := fmt.Errorf("%s statusCode %d", url, statusCode)
		ret := mapFetchResult{result: fetchResult{err: msg}}
		c <- ret
		return
	}

	var data map[string]interface{}
	err = json.Unmarshal(bytes, &data)
	if err != nil {
		msg := fmt.Errorf("%s returned junk: %v", url, err)
		ret := mapFetchResult{result: fetchResult{err: msg}}
		c <- ret
		return
	}

	c <- mapFetchResult{
		result: fetchResult{err: nil},
		data:   data,
	}
}

func fetchFeatureListFromOneEndpoint(ctx context.Context, c chan featureFetchResult, url string, token string, headers http.Header) {
	bytes, statusCode, _, err := fetchGet(ctx, url, token, headers)

	if err != nil {
		ret := featureFetchResult{result: fetchResult{err: err}}
		c <- ret
		return
	}

	if !httputil.StatusCodeOK(statusCode) {
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

	retchan := make(chan featureFetchResult)
	cds := clouddriverManager.getHealthyClouddriverURLs()

	for _, url := range cds {
		go fetchFeatureListFromOneEndpoint(req.Context(), retchan, combineURL(url.URL, req.RequestURI), url.token, req.Header)
	}

	ret := combineFeatureLists(retchan, len(cds))

	outjson, err := json.Marshal(ret)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusOK)
		httputil.CheckedWrite(w, outjson)
	}
}
