// anvilweb - AnviLLM static web frontend server
//
// Serves the AnviLLM web frontend (static files) and proxies /api/* requests
// to anvilwebgw (the HTTP gateway service). Run anvilwebgw separately to
// handle the 9P-connected API endpoints.
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
)

//go:embed static/*
var static embed.FS

var (
	addr    = flag.String("addr", ":8080", "HTTP server address")
	gwAddr  = flag.String("gw", "http://localhost:8081", "anvilwebgw gateway URL for /api/* requests")
)

func main() {
	flag.Parse()

	gwURL, err := url.Parse(*gwAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid gateway URL %q: %v\n", *gwAddr, err)
		os.Exit(1)
	}

	staticFS, err := fs.Sub(static, "static")
	if err != nil {
		log.Fatal(err)
	}

	// Reverse proxy for /api/* to anvilwebgw
	proxy := httputil.NewSingleHostReverseProxy(gwURL)

	mux := http.NewServeMux()
	mux.Handle("/api/", proxy)
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	log.Printf("Starting anvilweb on %s", *addr)
	log.Printf("Proxying /api/* to %s", *gwAddr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
