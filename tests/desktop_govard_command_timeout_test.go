package tests

import (
	"strings"
	"testing"
	"time"

	"govard/internal/desktop"
)

func TestGovardCommandTimeout(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want time.Duration
	}{
		{"up with flags", []string{"up", "--force-recreate", "--remove-orphans"}, 15 * time.Minute},
		{"up bare", []string{"up"}, 15 * time.Minute},
		{"bootstrap", []string{"bootstrap"}, 15 * time.Minute},
		{"env restart", []string{"env", "restart"}, 15 * time.Minute},
		{"env pull", []string{"env", "pull"}, 15 * time.Minute},
		{"svc restart", []string{"svc", "restart"}, 15 * time.Minute},
		{"svc pull", []string{"svc", "pull"}, 15 * time.Minute},
		{"svc up", []string{"svc", "up"}, 10 * time.Minute},
		{"env stop", []string{"env", "stop"}, 2 * time.Minute},
		{"svc stop", []string{"svc", "stop"}, 2 * time.Minute},
		{"init", []string{"init"}, 2 * time.Minute},
		{"debug action", []string{"debug", "xdebug"}, 2 * time.Minute},
		{"remote test", []string{"remote", "test", "staging"}, 2 * time.Minute},
		{"empty args", []string{}, 2 * time.Minute},
		{"nil args", nil, 2 * time.Minute},
		{"unknown subcommand", []string{"frobnicate"}, 2 * time.Minute},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := desktop.GovardCommandTimeoutForTest(tc.args); got != tc.want {
				t.Fatalf("timeout for %q = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestGovardCommandTimeoutErrorMentionsPartialState(t *testing.T) {
	err := desktop.SimulateGovardCommandTimeoutForTest([]string{"up", "--force-recreate"})
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "15m0s") {
		t.Fatalf("timeout error %q does not name the duration", msg)
	}
	if !strings.Contains(msg, "partially started") {
		t.Fatalf("timeout error %q does not warn about partial state", msg)
	}
}

func TestGovardCommandTimeoutErrorShortOpHasNoPartialStateHint(t *testing.T) {
	err := desktop.SimulateGovardCommandTimeoutForTest([]string{"env", "stop"})
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "2m0s") {
		t.Fatalf("timeout error %q does not name the duration", msg)
	}
	if strings.Contains(msg, "partially started") {
		t.Fatalf("timeout error %q must not warn about partial state for a short op", msg)
	}
}
