//go:build !ui

// Package web exposes the single-page app files.
package web

import (
	"io/fs"
	"testing/fstest"
)

// FS holds a placeholder page; build with -tags ui for the real app.
var FS fs.FS = fstest.MapFS{
	"index.html": {Data: []byte("<!doctype html><title>CeeMS</title><p>UI not built.")},
}
