package main

import (
	"encoding/base64"
	"io"
	"log"
	"net/http"

	"gopkg.in/yaml.v3"
)

// AccountStruct is a simple parse helper which contains only a small number
// of fields, specifically "account", so we can look at that field easily.
type AccountStruct struct {
	Account string `json:"account,omitempty"`
}

func (*srv) cloudOpsPost() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("content-type", "application/json")

		data, err := io.ReadAll(req.Body)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			log.Printf("Unable to read body in cloudOpsPost: %v", err)
			return
		}

		var list []map[string]AccountStruct
		err = yaml.Unmarshal(data, &list)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			log.Printf("Unable to parse body in cloudOpsPost: %v", err)
			return
		}

		request64 := base64.StdEncoding.EncodeToString(data)
		log.Printf("Request %s", request64)
		log.Printf("Request headers: %#v", req.Header)

		foundURLs := map[string]bool{}
		foundAccounts := map[string]bool{}

		for _, item := range list {
			for _, subitem := range item {
				if subitem.Account != "" {
					foundAccounts[subitem.Account] = true
					url, found := findAccountRoute(subitem.Account)
					if !found {
						log.Printf("Warning: account %s has no route", subitem.Account)
						continue
					}
					foundURLs[url] = true
				}
			}
		}

		foundAccountNames := keysForMapStringToBool(foundAccounts)

		if len(foundURLs) == 0 {
			log.Printf("Error: no routes found for any accounts in request: %v", foundAccountNames)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		if len(foundURLs) != 1 {
			log.Printf("WARNING: multiple routes found for accounts in request: %v.  Will try one at random.", foundAccountNames)
		}

		// will contain at least one element due to checking len(foundURLs) above
		foundURLNames := keysForMapStringToBool(foundURLs)

		target := combineURL(foundURLNames[0], req.RequestURI)
		responseBody, code, respHeaders, err := fetchPost(target, req.Header, data)
		response64 := base64.StdEncoding.EncodeToString(responseBody)
		log.Printf("Response: %d %s", code, response64)
		log.Printf("Response headers: %#v", respHeaders)

		if err != nil {
			log.Printf("Post error to %s: %v", target, err)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if !statusCodeOK(code) {
			w.WriteHeader(code)
			return
		}
		copyHeaders(w.Header(), respHeaders)
		w.WriteHeader(http.StatusOK)
		w.Write(responseBody)
	}
}

func keysForMapStringToBool(m map[string]bool) []string {
	ret := make([]string, 0, len(m))
	for k := range m {
		ret = append(ret, k)
	}
	return ret
}
