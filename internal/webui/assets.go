package webui

// Copyright 2025 The O²UL Authors
// This file is part of the O²UL blockchain library.

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
)

//go:embed distapp
var assets embed.FS

type metadata struct {
	BuildHash string `json:"buildHash"`
}

func Handler() http.Handler {
	sub, err := fs.Sub(assets, "distapp")
	if err != nil {
		return http.NotFoundHandler()
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			data, err := fs.ReadFile(sub, "index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func BuildHash() string {
	data, err := assets.ReadFile("distapp/meta.json")
	if err != nil {
		return ""
	}
	var meta metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return ""
	}
	return meta.BuildHash
}

func BuildLabel() string {
	hash := BuildHash()
	if hash == "" {
		return "frontend:unavailable"
	}
	if len(hash) > 8 {
		hash = hash[:8]
	}
	return fmt.Sprintf("frontend:%s", hash)
}