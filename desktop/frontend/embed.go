package frontend

import (
	"embed"
	"io/fs"
)

// dist holds the Vite build output (`make frontend`). dist/.gitkeep is
// committed so this package compiles on machines without Node; such a
// build serves an empty UI, which is why every desktop build target runs
// `make frontend` first.
//
//go:embed all:dist
var dist embed.FS

// Assets is the Vite build output rooted at dist/, served by the Wails asset server.
var Assets = mustSub(dist, "dist")

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
