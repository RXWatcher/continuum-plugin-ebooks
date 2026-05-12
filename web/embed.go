// Package web embeds the built SPA. The SPA itself is left as a minimal
// placeholder in v1 — the Go backend exposes the full REST API but the
// rich React reader is deferred (see plan Phases 20-24).
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var dist embed.FS

func FSEmbed() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic("web: " + err.Error())
	}
	return sub
}

func FS() http.FileSystem {
	return http.FS(FSEmbed())
}
