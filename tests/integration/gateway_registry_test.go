//go:build integration
// +build integration

package integration

import (
	"testing"
)

func TestIsRegistryUnavailable(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "registry auth failure",
			output: "pma Error unauthorized: authentication required\nError response from daemon: unauthorized: authentication required",
			want:   true,
		},
		{
			name:   "pull access denied",
			output: "Error response from daemon: pull access denied for caddy, repository does not exist",
			want:   true,
		},
		{
			name:   "rate limited",
			output: "toomanyrequests: You have reached your pull rate limit",
			want:   true,
		},
		{
			name:   "dns failure",
			output: "dial tcp: lookup registry-1.docker.io: no such host",
			want:   true,
		},
		{
			name:   "unhealthy container is a product failure",
			output: "svc up failed (1)\ncontainer deploy-code-only-php-1 is unhealthy",
			want:   false,
		},
		{
			name:   "compose error is a product failure",
			output: "svc up failed (1)\nservices.caddy Additional property proxy is not allowed",
			want:   false,
		},
		{
			name:   "empty output",
			output: "",
			want:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRegistryUnavailable(tc.output); got != tc.want {
				t.Fatalf("isRegistryUnavailable(%q) = %v, want %v", tc.output, got, tc.want)
			}
		})
	}
}
