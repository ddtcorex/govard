package tests

import (
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/engine/remote"
)

func TestProjectRemoteDBCredentialsDoesNotInjectConfigPrefixWithoutCapability(t *testing.T) {
	credentials := cmd.ProjectRemoteDBCredentialsForTest(
		engine.Config{TablePrefix: "wp_"},
		remote.RemoteDatabaseMetadata{Host: "db.example", Username: "wordpress", Database: "wordpress"},
		false,
	)
	if credentials.TablePrefix != "" {
		t.Errorf("TablePrefix = %q, want empty when framework does not opt in", credentials.TablePrefix)
	}
}

func TestProjectRemoteDBCredentialsUsesRemotePrefixBeforeOptInConfigPrefix(t *testing.T) {
	t.Run("remote prefix wins", func(t *testing.T) {
		credentials := cmd.ProjectRemoteDBCredentialsForTest(
			engine.Config{TablePrefix: "local_"},
			remote.RemoteDatabaseMetadata{Host: "db.example", Username: "magento", Database: "magento", TablePrefix: "remote_"},
			true,
		)
		if credentials.TablePrefix != "remote_" {
			t.Errorf("TablePrefix = %q, want remote_", credentials.TablePrefix)
		}
	})

	t.Run("opt-in config prefix is fallback", func(t *testing.T) {
		credentials := cmd.ProjectRemoteDBCredentialsForTest(
			engine.Config{TablePrefix: "local_"},
			remote.RemoteDatabaseMetadata{Host: "db.example", Username: "prestashop", Database: "prestashop"},
			true,
		)
		if credentials.TablePrefix != "local_" {
			t.Errorf("TablePrefix = %q, want local_", credentials.TablePrefix)
		}
	})
}
