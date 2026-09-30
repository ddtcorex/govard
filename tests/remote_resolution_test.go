package tests

import (
	"errors"
	"govard/internal/cli"
	"govard/internal/cmd"
	"govard/internal/deploy"
	"govard/internal/engine"
	"strings"
	"testing"
)

func TestResolveAutoRemote(t *testing.T) {
	tests := []struct {
		name      string
		config    engine.Config
		requested string
		want      string
		wantErr   string
	}{
		{
			name: "Explicitly requested exists",
			config: engine.Config{
				Remotes: map[string]engine.RemoteConfig{
					"prod": {Host: "prod.com"},
				},
			},
			requested: "prod",
			want:      "prod",
		},
		{
			name: "Explicitly requested exists as alias",
			config: engine.Config{
				Remotes: map[string]engine.RemoteConfig{
					"development": {Host: "dev.com"},
				},
			},
			requested: "dev",
			want:      "development",
		},
		{
			name: "Explicitly requested missing",
			config: engine.Config{
				Remotes: map[string]engine.RemoteConfig{
					"staging": {Host: "stg.com"},
				},
			},
			requested: "prod",
			wantErr:   "remote 'prod' is not configured",
		},
		{
			name: "Auto-select staging (primary focus)",
			config: engine.Config{
				Remotes: map[string]engine.RemoteConfig{
					"staging":     {Host: "stg.com"},
					"development": {Host: "dev.com"},
				},
			},
			requested: "",
			want:      "staging",
		},
		{
			name: "Auto-select staging via alias",
			config: engine.Config{
				Remotes: map[string]engine.RemoteConfig{
					"stg":         {Host: "stg.com"},
					"development": {Host: "dev.com"},
				},
			},
			requested: "",
			want:      "stg",
		},
		{
			name: "Auto-select dev when staging missing",
			config: engine.Config{
				Remotes: map[string]engine.RemoteConfig{
					"dev": {Host: "dev.com"},
				},
			},
			requested: "",
			want:      "dev",
		},
		{
			name: "Auto-select development alias when staging missing",
			config: engine.Config{
				Remotes: map[string]engine.RemoteConfig{
					"development": {Host: "dev.com"},
				},
			},
			requested: "",
			want:      "development",
		},
		{
			name: "Custom name explicitly requested",
			config: engine.Config{
				Remotes: map[string]engine.RemoteConfig{
					"qa": {Host: "qa.com"},
				},
			},
			requested: "qa",
			want:      "qa",
		},
		{
			name: "Custom name preprod explicitly requested",
			config: engine.Config{
				Remotes: map[string]engine.RemoteConfig{
					"preprod": {Host: "preprod.com"},
					"staging": {Host: "stg.com"},
				},
			},
			requested: "preprod",
			want:      "preprod",
		},
		{
			name: "Custom name not found returns error",
			config: engine.Config{
				Remotes: map[string]engine.RemoteConfig{
					"qa": {Host: "qa.com"},
				},
			},
			requested: "demo",
			wantErr:   "remote 'demo' is not configured",
		},
		{
			name: "Neither exists",
			config: engine.Config{
				Remotes: map[string]engine.RemoteConfig{
					"production": {Host: "prod.com"},
				},
			},
			requested: "",
			wantErr:   "no remote environment found (tried staging, dev)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cmd.ResolveAutoRemote(tt.config, tt.requested)
			if tt.wantErr != "" {
				if err == nil {
					t.Errorf("ResolveAutoRemote() expected error %q, got nil", tt.wantErr)
					return
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("ResolveAutoRemote() error = %v, wantErr %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("ResolveAutoRemote() unexpected error: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("ResolveAutoRemote() got = %q, want %q", got, tt.want)
			}
		})
	}
}

// A name the project does not configure is a configuration error, and the class
// has to be decided here rather than by each caller: `sync` and `bootstrap`
// return this error untouched, so a plain error leaves both reporting exit 1 for
// a file the operator has to edit.
func TestResolveAutoRemoteClassifiesAnUnknownRemote(t *testing.T) {
	config := engine.Config{
		ProjectName: "sample-project",
		Remotes:     engine.RemoteConfigMap{"staging": {Host: "stg.example.com"}},
	}

	_, err := cmd.ResolveAutoRemote(config, "prod")
	if !errors.Is(err, deploy.ErrUnknownRemote) {
		t.Fatalf("ResolveAutoRemote() err = %v, want deploy.ErrUnknownRemote", err)
	}
	if code := cli.Code(err); code != cli.CodeConfig {
		t.Errorf("cli.Code(err) = %d, want %d (a configuration error)", code, cli.CodeConfig)
	}
	// The sentinel supplies the class, not a new sentence: the wording the CLI
	// has always printed is still there, verbatim.
	if want := "remote 'prod' is not configured"; !strings.Contains(err.Error(), want) {
		t.Errorf("ResolveAutoRemote() err = %v, want it to still say %q", err, want)
	}
}

// The other half of the same boundary. Nothing was requested here, so there is no
// name to call unconfigured — the command is telling an operator that nothing in
// their project offers a source, and that stays an execution failure.
func TestResolveAutoRemoteWithNoEnvironmentIsNotAnUnknownRemote(t *testing.T) {
	config := engine.Config{
		ProjectName: "sample-project",
		Remotes:     engine.RemoteConfigMap{"production": {Host: "prod.example.com"}},
	}

	_, err := cmd.ResolveAutoRemote(config, "")
	if err == nil {
		t.Fatal("ResolveAutoRemote() err = <nil>, want the no-environment message")
	}
	if errors.Is(err, deploy.ErrUnknownRemote) {
		t.Errorf("ResolveAutoRemote() err = %v, want it not to carry deploy.ErrUnknownRemote: no name was requested", err)
	}
}
