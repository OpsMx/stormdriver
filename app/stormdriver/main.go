package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

type srv struct {
	listenPort     uint16
	destinationURL string
	Insecure       bool
}

const httpPort = 8090
const dialTimeout = 15 * time.Second
const clientTimeout = 15 * time.Second
const tlsHandshakeTimeout = 15 * time.Second
const responseHeaderTimeout = 15 * time.Second
const maxIdleConns = 5

func (*srv) headers() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		for name, headers := range req.Header {
			for _, h := range headers {
				fmt.Fprintf(w, "%v: %v\n", name, h)
			}
		}
	}
}

func transfer(destination io.WriteCloser, source io.ReadCloser) {
	defer destination.Close()
	defer source.Close()
	io.Copy(destination, source)
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
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
	Request  tracerHTTP `json:"request,omitempty"`
	Response tracerHTTP `json:"response,omitempty"`
}

func (s *srv) redirect() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {

		dialer := net.Dialer{Timeout: dialTimeout}

		client := &http.Client{
			Timeout: clientTimeout,
			Transport: &http.Transport{
				Dial:                  dialer.Dial,
				DialContext:           dialer.DialContext,
				TLSHandshakeTimeout:   tlsHandshakeTimeout,
				ResponseHeaderTimeout: responseHeaderTimeout,
				ExpectContinueTimeout: time.Second,
				MaxIdleConns:          maxIdleConns,
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

		target := s.destinationURL + req.RequestURI
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
		copyHeader(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			log.Printf("%v", err)
			return
		}

		t := tracer{
			Request: tracerHTTP{
				Body:    base64.StdEncoding.EncodeToString(reqBody),
				Headers: req.Header,
			},
			Response: tracerHTTP{
				Body:       base64.StdEncoding.EncodeToString(respBody),
				Headers:    resp.Header,
				StatusCode: resp.StatusCode,
			},
		}
		json, _ := json.Marshal(t)

		log.Printf("%s", json)
		w.Write(respBody)
	}
}

func (s *srv) routes(mux *http.ServeMux) {
	mux.HandleFunc("/_headers", s.headers())
	mux.HandleFunc("/", s.redirect())
}

func main() {
	s := &srv{
		listenPort:     httpPort,
		destinationURL: "http://spin-clouddriver-caching:7002",
	}
	mux := http.NewServeMux()
	s.routes(mux)
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.listenPort),
		Handler: mux,
		// Disable HTTP/2.
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}
	log.Fatal(srv.ListenAndServe())
}
