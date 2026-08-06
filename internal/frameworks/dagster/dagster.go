package dagster

import (
	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	return types.FrameworkDefinition{
		Name:        "dagster",
		DisplayName: "Dagster",
		Config:      config,
		Manifest:    manifest,
		DefaultDBCredentials: types.DefaultDBCredentials{
			Port:     conventions.PostgresPort,
			Username: conventions.DefaultDagsterDBUser,
			Password: conventions.DefaultDagsterDBPass,
			Database: conventions.DefaultDagsterDBName,
		},
		Detect: engine.DetectionSpec{
			FilePaths: []string{"workspace.yaml", "dagster.yaml"},
		},
		ToolCommands: []types.ToolCommand{
			{Name: "dagster", Short: "Run Dagster CLI commands", Binary: "dagster"},
		},
		Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
			return NewDagsterBootstrap(opts)
		},
		FreshInstall:                freshInstall,
		FreshInstallManagesOwnEnvUp: true,
		FreshInstallNeedsDomain:     false,
		SupportsFreshInstall:        true,
		SupportsBootstrap:           true,
	}
}
