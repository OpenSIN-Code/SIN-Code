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

func templateSub() fs.FS {
	sub, err := fs.Sub(templateFS, "templates")
	if err != nil {
		panic(err)
	}
	return sub
}

func staticSub() fs.FS {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	return sub
}
