// SPDX-License-Identifier: MIT
// Purpose: embed HTML templates for the sin-code web UI.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed templates/*
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

var (
	// test hooks for error paths that are impossible to hit with the embedded FS.
	templateFSSubHook = func() (fs.FS, error) { return fs.Sub(templateFS, "templates") }
	staticFSSubHook   = func() (fs.FS, error) { return fs.Sub(staticFS, "static") }
)

func templateSub() fs.FS {
	sub, err := templateFSSubHook()
	if err != nil {
		panic(err)
	}
	return sub
}

func staticSub() fs.FS {
	sub, err := staticFSSubHook()
	if err != nil {
		panic(err)
	}
	return sub
}
