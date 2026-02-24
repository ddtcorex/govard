package blueprints

import (
	"embed"
	"io/fs"
)

//go:embed all:files
var files embed.FS

// FS is the embedded blueprints filesystem
var FS fs.FS

func init() {
	var err error
	FS, err = fs.Sub(files, "files")
	if err != nil {
		panic(err)
	}
}
