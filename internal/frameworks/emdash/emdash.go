package emdash

import (
	"text/template"

	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	return types.FrameworkDefinition{
		Name:             "emdash",
		DisplayName:      "Emdash",
		Config:           config,
		Manifest:         manifest,
		NodeImageFlavor:  "standard",
		DefaultAdminPath: "_emdash/admin",
		DefaultDBCredentials: types.DefaultDBCredentials{
			Port:     conventions.MySQLPort,
			Username: conventions.DefaultDBUser,
			Password: conventions.DefaultDBPass,
			Database: conventions.DefaultDBName,
		},
		Detect: engine.DetectionSpec{
			PackageJSONDeps: []string{"emdash"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return NewEmdashBootstrap(opts)
		},
		FreshInstall:         freshInstall,
		SupportsFreshInstall: true,
		TemplateFuncs:        template.FuncMap{"emdashRuntimeCommand": BuildRuntimeCommand},
	}
}
