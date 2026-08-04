package cakephp

import (
	"embed"
	"io/fs"

	"govard/internal/blueprints"
)

//go:embed all:blueprint
var blueprintFiles embed.FS

// BlueprintFS is cakephp's embedded blueprint sub-filesystem.
var BlueprintFS fs.FS

func init() {
	var err error
	BlueprintFS, err = fs.Sub(blueprintFiles, "blueprint")
	if err != nil {
		panic(err)
	}

	blueprints.RegisterFrameworkMount(blueprints.FrameworkMount{
		Framework:     "cakephp",
		FS:            BlueprintFS,
		HasDir:        false,
		NginxTemplate: "cakephp.conf",
	})
}
