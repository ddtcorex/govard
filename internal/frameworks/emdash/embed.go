package emdash

import (
	"embed"
	"io/fs"

	"govard/internal/blueprints"
)

//go:embed all:blueprint
var blueprintFiles embed.FS

var BlueprintFS fs.FS

func init() {
	var err error
	BlueprintFS, err = fs.Sub(blueprintFiles, "blueprint")
	if err != nil {
		panic(err)
	}

	blueprints.RegisterFrameworkMount(blueprints.FrameworkMount{
		Framework: "emdash",
		FS:        BlueprintFS,
		HasDir:    true,
	})
}
