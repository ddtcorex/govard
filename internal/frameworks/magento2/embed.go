package magento2

import (
	"embed"
	"io/fs"

	"govard/internal/blueprints"
)

// blueprintFiles holds magento2's own blueprint assets: the varnish VCL
// (relocated from internal/blueprints/files/magento2/varnish/default.vcl)
// and the nginx vhost template (relocated from
// internal/blueprints/files/support/nginx/templates/magento2.conf).
//
//go:embed all:blueprint
var blueprintFiles embed.FS

// BlueprintFS is magento2's embedded blueprint sub-filesystem, rooted so
// paths match the legacy layout under internal/blueprints/files/magento2/
// (e.g. "varnish/default.vcl", "magento2.conf" - not
// "blueprint/varnish/default.vcl").
var BlueprintFS fs.FS

func init() {
	var err error
	BlueprintFS, err = fs.Sub(blueprintFiles, "blueprint")
	if err != nil {
		panic(err)
	}

	blueprints.RegisterFrameworkMount(blueprints.FrameworkMount{
		Framework:     "magento2",
		FS:            BlueprintFS,
		HasDir:        true,
		NginxTemplate: "magento2.conf",
	})
}
