// Package custom describes a deliberately unopinionated project stack.
package custom

import (
	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
	return types.FrameworkDefinition{
		Name:        "custom",
		DisplayName: "Custom",
		Config:      config,
		Manifest:    manifest,
		DefaultDBCredentials: types.DefaultDBCredentials{
			Port:     conventions.MySQLPort,
			Username: conventions.DefaultDBUser,
			Password: conventions.DefaultDBPass,
			Database: conventions.DefaultDBName,
		},
		Detect: engine.DetectionSpec{},
	}
}
