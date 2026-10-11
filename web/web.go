// Package web embeds the built browser app so the server ships as one binary.
//
// The app's source is in src/ (React + TypeScript); `npm run build` writes it
// to dist/. The Dockerfile runs that build before compiling Go.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var files embed.FS

// Static returns the files under dist/, served at the site root.
func Static() fs.FS {
	sub, err := fs.Sub(files, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
