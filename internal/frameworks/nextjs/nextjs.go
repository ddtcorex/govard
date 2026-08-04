package nextjs

import (
	"text/template"

	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	return types.FrameworkDefinition{
		Name:            "nextjs",
		DisplayName:     "Next.js",
		Config:          config,
		Manifest:        manifest,
		NodeImageFlavor: "standard",
		DefaultDBCredentials: types.DefaultDBCredentials{
			Port:     conventions.MySQLPort,
			Username: conventions.DefaultDBUser,
			Password: conventions.DefaultDBPass,
			Database: conventions.DefaultDBName,
		},
		Detect: engine.DetectionSpec{
			PackageJSONDeps: []string{"next"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return NewNextJSBootstrap(opts)
		},
		FreshInstall:         freshInstall,
		SupportsFreshInstall: true,
		TemplateFuncs:        template.FuncMap{"nextjsRuntimeCommand": BuildRuntimeCommand},
	}
}
