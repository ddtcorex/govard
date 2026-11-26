package tests

import (
	"errors"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/engine/remote"
)

func TestRemotePkgBuildSSHArgs(t *testing.T) {
	cfg := engine.RemoteConfig{
		Port: 2202,
		Auth: engine.RemoteAuth{
			StrictHostKey:  true,
			KnownHostsFile: "/tmp/known_hosts",
			KeyPath:        "/tmp/id_ed25519",
		},
	}
	args := remote.BuildSSHArgs("staging", cfg, true, false)
	joined := strings.Join(args, " ")
	for _, want := range []string{"StrictHostKeyChecking=yes", "UserKnownHostsFile=/tmp/known_hosts", "-p 2202", "-A", "-i /tmp/id_ed25519"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in args: %s", want, joined)
		}
	}
}

func TestRemotePkgClassifyFailure(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		output   string
		category string
	}{
		{name: "host key", output: "Host key verification failed", category: remote.FailureCategoryHostKey},
		{name: "auth", output: "Permission denied (publickey)", category: remote.FailureCategoryAuth},
		{name: "network", output: "Could not resolve hostname", category: remote.FailureCategoryNetwork},
		{name: "dependency", output: "rsync: not found", category: remote.FailureCategoryDependency},
		{name: "permission", output: "Operation not permitted", category: remote.FailureCategoryPermission},
		{name: "unknown", err: errors.New("boom"), output: "", category: remote.FailureCategoryUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			details := remote.ClassifyFailure(tc.err, tc.output)
			if details.Category != tc.category {
				t.Fatalf("expected category %s, got %s", tc.category, details.Category)
			}
		})
	}
}
