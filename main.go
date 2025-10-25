package main

import (
	"flag"
	"fmt"
	"log"
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

	// Handler wraps proxy.ServeHTTP and implements log levels.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
