package main

import (
	"bytes"
	"embed"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
)

//go:embed ui/*
var uiFS embed.FS

// serveFile from filesystem in devmode or from embedded fs in prod mode
func (s *ProxyServer) serveFile(w http.ResponseWriter, r *http.Request, filePath string, contentType string) {
	if s.devMode {
		log.Print(filePath)
		http.ServeFile(w, r, filePath)
		return
	}

	// Serve from embedded file system
	f, err := uiFS.Open(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	fi, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, filePath, fi.ModTime(), bytes.NewReader(content))
}

// serve /ui (index.html)
func (s *ProxyServer) handleUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/ui" || r.URL.Path == "/ui/" {
		s.serveFile(w, r, "ui/index.html", "")
		return
	}

	path := filepath.Join("ui/", filepath.Clean(strings.TrimPrefix(r.URL.Path, "/ui")))
	s.serveFile(w, r, path, "")
}

// serve favicon.ico
func (s *ProxyServer) handleFavIcon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=86400")
	s.serveFile(w, r, "ui/favicon.ico", "image/x-icon")
}
