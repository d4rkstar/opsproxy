// Licensed to the Apache Software Foundation (ASF) under one
// or more contributor license agreements.  See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership.  The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
)

var firstLogged int32

func main() {
	listenPort := flag.Int("listen-port", 80, "port to listen on")
	targetHost := flag.String("target-host", "127.0.0.1", "target host (may include scheme)")
	targetPort := flag.Int("target-port", 8080, "target port")
	logLevel := flag.String("log-level", "info", "log level: verbose|info|error")
	flag.Parse()

	// Build target URL. If user provided a scheme in targetHost, respect it.
	var target string
	if strings.HasPrefix(*targetHost, "http://") || strings.HasPrefix(*targetHost, "https://") {
		// If host already contains a port, don't append one
		if strings.Contains(*targetHost, ":") {
			target = *targetHost
		} else {
			target = fmt.Sprintf("%s:%d", *targetHost, *targetPort)
		}
	} else {
		target = fmt.Sprintf("http://%s:%d", *targetHost, *targetPort)
	}

	u, err := url.Parse(target)
	if err != nil {
		log.Fatalf("invalid target URL %q: %v", target, err)
	}

	proxy := httputil.NewSingleHostReverseProxy(u)

	// ErrorHandler logs errors and returns 502
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		log.Printf("ERROR: forwarding %s %s from %s: %v", r.Method, r.URL.String(), r.RemoteAddr, e)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	// Handler wraps proxy.ServeHTTP and implements log levels. It also
	// handles connection upgrades (websocket/other) by proxying the raw
	// connection between client and backend so streaming works.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Detect upgrade requests (e.g. websocket)
		if isUpgradeRequest(r) {
			if err := proxyUpgrade(w, r, u); err != nil {
				log.Printf("ERROR: upgrade proxy %s %s from %s: %v", r.Method, r.URL.String(), r.RemoteAddr, err)
				http.Error(w, "Bad Gateway", http.StatusBadGateway)
			}
			return
		}
		lvl := strings.ToLower(*logLevel)
		switch lvl {
		case "verbose":
			log.Printf("FORWARD: %s %s <- %s", r.Method, r.URL.String(), r.RemoteAddr)
		case "info":
			if atomic.CompareAndSwapInt32(&firstLogged, 0, 1) {
				log.Printf("FIRST: %s %s <- %s", r.Method, r.URL.String(), r.RemoteAddr)
			}
		case "error":
			// do not log normal requests
		default:
			// unknown level -> default to info behavior
			if atomic.CompareAndSwapInt32(&firstLogged, 0, 1) {
				log.Printf("FIRST: %s %s <- %s", r.Method, r.URL.String(), r.RemoteAddr)
			}
		}

		// Forward the request
		proxy.ServeHTTP(w, r)
	})

	http.Handle("/", handler)

	addr := fmt.Sprintf(":%d", *listenPort)
	log.Printf("Proxy listening on %s -> %s (log level=%s)", addr, u.String(), *logLevel)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

// isUpgradeRequest returns true if the incoming request asks to upgrade the
// connection (commonly used for WebSockets).
func isUpgradeRequest(r *http.Request) bool {
	up := r.Header.Get("Connection")
	if strings.EqualFold(up, "upgrade") {
		return true
	}
	// Some clients send multiple header values, check for substring.
	if strings.Contains(strings.ToLower(up), "upgrade") {
		return true
	}
	return r.Header.Get("Upgrade") != ""
}

// proxyUpgrade performs a raw TCP proxy between the client and the backend
// for upgrade requests. It dials the backend using the scheme/host from u
// and forwards bytes in both directions.
func proxyUpgrade(w http.ResponseWriter, r *http.Request, u *url.URL) error {
	// Hijack client connection
	hj, ok := w.(http.Hijacker)
	if !ok {
		return fmt.Errorf("response writer does not support hijacking")
	}
	clientConn, clientBuf, err := hj.Hijack()
	if err != nil {
		return fmt.Errorf("hijack failed: %w", err)
	}
	defer func() {
		_ = clientConn.Close()
	}()

	// Connect to backend
	backendAddr := u.Host
	// Ensure host has a port if missing (Url.Parse guarantees when scheme present)
	var backendConn net.Conn
	if u.Scheme == "https" {
		backendConn, err = tls.Dial("tcp", backendAddr, &tls.Config{InsecureSkipVerify: true})
	} else {
		backendConn, err = net.Dial("tcp", backendAddr)
	}
	if err != nil {
		return fmt.Errorf("dial backend %s: %w", backendAddr, err)
	}
	defer func() { _ = backendConn.Close() }()

	// Write the request line and headers to the backend (preserve original)
	if err := r.Write(backendConn); err != nil {
		return fmt.Errorf("writing request to backend: %w", err)
	}

	// Now proxy bytes between clientConn and backendConn
	errc := make(chan error, 2)
	go func() {
		_, e := io.Copy(backendConn, clientBuf)
		errc <- e
	}()
	go func() {
		_, e := io.Copy(clientConn, backendConn)
		errc <- e
	}()

	// Wait for one side to finish or error
	e := <-errc
	return e
}
