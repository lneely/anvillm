// anvilweb - AnviLLM static web frontend server
//
// Serves the AnviLLM web frontend (static files) and proxies /api/* requests
// to anvilwebgw (the HTTP gateway service). Run anvilwebgw separately to
// handle the 9P-connected API endpoints.
package main

import (
	"anvillm/internal/logging"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"

	"go.uber.org/zap"
)

//go:embed static/*
var static embed.FS

var (
	addr    = flag.String("addr", ":8080", "HTTP server address")
	gwAddr  = flag.String("gw", "http://localhost:8081", "anvilwebgw gateway URL for /api/* requests")
)

func main() {
	if err := logging.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logging: %v\n", err)
		os.Exit(1)
	}
	defer logging.Logger().Sync()

	flag.Parse()

	gwURL, err := url.Parse(*gwAddr)
	if err != nil {
		logging.Logger().Fatal("invalid gateway URL", zap.String("url", *gwAddr), zap.Error(err))
	}

	staticFS, err := fs.Sub(static, "static")
	if err != nil {
		logging.Logger().Fatal("failed to load static files", zap.Error(err))
	}

	// Reverse proxy for /api/* to anvilwebgw
	proxy := httputil.NewSingleHostReverseProxy(gwURL)

	mux := http.NewServeMux()
	mux.Handle("/api/", proxy)
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	logging.Logger().Info("starting anvilweb", zap.String("addr", *addr), zap.String("gateway", *gwAddr))
	if err := http.ListenAndServe(*addr, mux); err != nil {
		logging.Logger().Fatal("server error", zap.Error(err))
	}
}
