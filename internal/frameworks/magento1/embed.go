package magento1

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
		Framework:     "magento1",
		FS:            BlueprintFS,
		HasDir:        true,
		NginxTemplate: "magento1.conf",
	})
}
