package django

import (
	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	return types.FrameworkDefinition{
		Name:        "django",
		DisplayName: "Django",
		Config:      config,
		Manifest:    manifest,
		DefaultDBCredentials: types.DefaultDBCredentials{
			Port:     conventions.PostgresPort,
			Username: conventions.DefaultDjangoDBUser,
			Password: conventions.DefaultDjangoDBPass,
			Database: conventions.DefaultDjangoDBName,
		},
		Detect: engine.DetectionSpec{
			FilePaths: []string{"manage.py"},
		},
		ToolCommands: []types.ToolCommand{
			{Name: "manage", Short: "Run Django management commands", Binary: "python", PrependArgs: []string{"manage.py"}},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return NewDjangoBootstrap(opts)
		},
		FreshInstall:                freshInstall,
		FreshInstallManagesOwnEnvUp: true,
		FreshInstallNeedsDomain:     true,
		SupportsFreshInstall:        true,
		SupportsBootstrap:           true,
	}
}
