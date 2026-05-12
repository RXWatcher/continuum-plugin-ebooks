// Package web embeds the built SPA so the Go binary contains a single
// artifact suitable for upload via continuum's plugin uploader.
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
