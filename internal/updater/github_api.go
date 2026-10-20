package updater

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// GitHubTokenEnvVar is the govard-specific token override. It wins over
	// GitHubTokenGenericEnvVar when both are set (same precedence rule as
	// eget's EGET_GITHUB_TOKEN over GITHUB_TOKEN).
	GitHubTokenEnvVar = "GOVARD_GITHUB_TOKEN"
	// GitHubTokenGenericEnvVar is the community-standard token variable
	// shared with gh, eget, cargo-binstall and friends.
	GitHubTokenGenericEnvVar = "GITHUB_TOKEN"
)

// GitHubAuthToken returns the configured GitHub API token, if any.
// A value of the form "@/path/to/file" reads the token from that file
// (surrounding whitespace is trimmed). Missing files, empty values and
// unset variables all report "" — token auth stays strictly optional.
func GitHubAuthToken() string {
	if token := resolveGitHubTokenValue(os.Getenv(GitHubTokenEnvVar)); token != "" {
		return token
	}
	return resolveGitHubTokenValue(os.Getenv(GitHubTokenGenericEnvVar))
}

func resolveGitHubTokenValue(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "@") {
		return value
	}
	path := strings.TrimSpace(strings.TrimPrefix(value, "@"))
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// ApplyGitHubAuth attaches the configured token (if any) to req, but only
// for api.github.com requests: a REST token is useless on github.com web
// and download URLs, so it is never sent there. The token value itself is
// never logged; error paths must keep that property.
func ApplyGitHubAuth(req *http.Request) {
	if req == nil || req.URL == nil {
		return
	}
	if !strings.EqualFold(req.URL.Hostname(), "api.github.com") {
		return
	}
	if token := GitHubAuthToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// GitHubAPIError builds a human-actionable error for a failed GitHub API
// call. Exhausted-quota responses (403/429 carrying an empty-rate budget)
// name the reset time and the two escape hatches — pinning --version or
// setting a token — instead of surfacing a bare status code. Anything else
// keeps the historical "failed with status N" shape.
func GitHubAPIError(requestURL string, status int, header http.Header, snippet string) error {
	if isGitHubQuotaError(status, header) {
		message := fmt.Sprintf("GitHub API rate limit exceeded for %s (status %d)", requestURL, status)
		if reset, ok := githubQuotaReset(header); ok {
			message += fmt.Sprintf("; quota resets at %s", reset)
		}
		message += ". Re-run with an explicit version to skip the lookup " +
			"(govard self-update --version vX.Y.Z), or set " +
			GitHubTokenEnvVar + " (or " + GitHubTokenGenericEnvVar + ") to raise the quota."
		return fmt.Errorf("%s", message)
	}
	if snippet = strings.TrimSpace(snippet); snippet != "" {
		return fmt.Errorf("request %s failed with status %d: %s", requestURL, status, snippet)
	}
	return fmt.Errorf("request %s failed with status %d", requestURL, status)
}

func isGitHubQuotaError(status int, header http.Header) bool {
	switch status {
	case http.StatusTooManyRequests:
		return true
	case http.StatusForbidden:
		return strings.TrimSpace(header.Get("X-RateLimit-Remaining")) == "0"
	default:
		return false
	}
}

func githubQuotaReset(header http.Header) (string, bool) {
	raw := strings.TrimSpace(header.Get("X-RateLimit-Reset"))
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seconds <= 0 {
		return "", false
	}
	reset := time.Unix(seconds, 0)
	formatted := reset.UTC().Format("2006-01-02 15:04 UTC")
	if remaining := time.Until(reset).Round(time.Minute); remaining > 0 {
		return fmt.Sprintf("%s (in ~%s)", formatted, remaining), true
	}
	return formatted, true
}
