package verify

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"govard/internal/conventions"
)

// probeTimeout bounds one probe attempt. These items ask a host route whether
// something answers; a dropped packet must not hold a phase open.
const probeTimeout = 5 * time.Second

// probeBodyLimit caps how much of an answer is read. A search health payload is
// a few hundred bytes, and whatever answers the route is not necessarily the
// search engine.
const probeBodyLimit = 64 << 10

// probeHTTPStatus performs one GET and reports the status code, following
// redirects.
//
// It uses the default transport on purpose: TLS verification stays enabled. The
// caller that falls back to http:// does so because TLS could not be
// established at all, and the fix for an untrusted local CA is
// `govard doctor trust` — never a disabled check.
func probeHTTPStatus(ctx context.Context, url string) (int, error) {
	status, _, err := probeHTTP(ctx, url)
	return status, err
}

// probeHTTPFake is a test hook, the probe-side counterpart of execGovardFake:
// when set, probeHTTP answers from it and nothing leaves the process. P2-13 and
// P4-11 do their own HTTP, so a test that drives whole phases end-to-end cannot
// keep them hermetic through the exec fake alone.
var probeHTTPFake func(ctx context.Context, url string) (int, []byte, error)

// SetProbeHTTPFakeForTest installs a hermetic probe for tests. Pass nil to
// clear it.
func SetProbeHTTPFakeForTest(fn func(ctx context.Context, url string) (int, []byte, error)) {
	probeHTTPFake = fn
}

// probeHTTP is the one GET both probes share: bounded by probeTimeout and
// probeBodyLimit, with TLS verification left on.
func probeHTTP(ctx context.Context, url string) (int, []byte, error) {
	if probeHTTPFake != nil {
		return probeHTTPFake(ctx, url)
	}

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, probeBodyLimit))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

// tlsNotUsable reports whether an https:// attempt died before any HTTP
// response because TLS could not be established with this peer: an untrusted
// local CA (x509 errors, wrapped in tls.CertificateVerificationError), a port
// that does not speak TLS at all (tls.RecordHeaderError, or net/http's
// ErrSchemeMismatch when the peer answers a ClientHello with plain HTTP), or a
// handshake alert.
//
// Only this class licenses retrying the same domain over plain http://. A
// refused connection, a DNS failure and an HTTP error status are different
// problems, and retrying them would report "the site answers" for a site that
// does not — the false green the item exists to prevent.
func tlsNotUsable(err error) bool {
	var (
		certErr   *tls.CertificateVerificationError
		recordErr tls.RecordHeaderError
		alertErr  tls.AlertError
		unknownCA x509.UnknownAuthorityError
		hostErr   x509.HostnameError
		invalid   x509.CertificateInvalidError
	)
	return errors.As(err, &certErr) ||
		errors.As(err, &recordErr) ||
		errors.As(err, &alertErr) ||
		errors.As(err, &unknownCA) ||
		errors.As(err, &hostErr) ||
		errors.As(err, &invalid) ||
		errors.Is(err, http.ErrSchemeMismatch)
}

// probeSite answers P2-13: does the project's own domain answer?
//
// The item used to run `govard tool curl -k …` twice. `tool` is a fixed
// subcommand registry with no curl (internal/cmd/frameworks.go), so every run
// ended on "unknown flag" with exit 2; and even a registered curl would have
// failed, because the app container cannot resolve the project's own domain —
// the docker aliases cover `mail` and the search containers, and <domain> is
// mapped only for linked projects. The probe belongs on the host, which is
// where `govard verify` runs.
//
// It tries https://<domain>/ first and falls back to http://<domain>/ only when
// the TLS handshake could not be established (see tlsNotUsable). The evidence
// names the scheme that answered, so a green row never hides which one it was.
func probeSite(ctx context.Context, domain string) Evidence {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		// Not a red, and above all not a guessed target: the item cannot say
		// anything about "the site" without a domain, so the row stays visible
		// with the reason. P2-13 used to substitute localhost here, which
		// probed the host's own :80/:443 and reported whatever answered as the
		// project's site.
		return Skip("no domain configured: this item probes the project's own domain")
	}

	httpsURL := "https://" + domain + "/"
	status, err := probeHTTPStatus(ctx, httpsURL)
	if err == nil {
		return siteEvidence("", httpsURL, status, "https")
	}
	if !tlsNotUsable(err) {
		return Evidence{ExitCode: 1, OutputExcerpt: fmt.Sprintf("GET %s failed: %v", httpsURL, err)}
	}

	httpURL := "http://" + domain + "/"
	status, httpErr := probeHTTPStatus(ctx, httpURL)
	note := fmt.Sprintf("TLS handshake failed (%v); ", err)
	if httpErr != nil {
		return Evidence{ExitCode: 1, OutputExcerpt: fmt.Sprintf("%sGET %s failed: %v", note, httpURL, httpErr)}
	}
	return siteEvidence(note, httpURL, status, "http")
}

// siteEvidence renders one answered attempt. 2xx and 3xx mean the site
// answered; anything else is a red that names the status it got.
func siteEvidence(note, url string, status int, scheme string) Evidence {
	ev := Evidence{OutputExcerpt: fmt.Sprintf("%sGET %s -> %d (scheme %s)", note, url, status, scheme)}
	if status < 200 || status >= 400 {
		ev.ExitCode = 1
		ev.OutputExcerpt += ": not 2xx/3xx"
	}
	return ev
}

// searchHealthURL is the host route the shared proxy publishes for a project's
// search container: plain HTTP, the project's own domain, on the search port
// (docs/workflows/ssl-and-domains.md). The item this replaces hard-coded
// http://localhost:9200, which is the container's address inside the compose
// network — nothing on the host answers it — while its title promised the
// domain route.
func searchHealthURL(domain string) string {
	return "http://" + strings.TrimSpace(domain) + ":" + strconv.Itoa(conventions.SearchPort) + "/_cluster/health"
}

// probeSearchHealth answers P4-11: does the host route reach the search engine?
func probeSearchHealth(ctx context.Context, domain string) Evidence {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		// Same reasoning as probeSite: without a domain there is no host route
		// to probe, and guessing one would report on another project's search
		// container.
		return Skip("no domain configured: this item probes the project's own domain")
	}

	url := searchHealthURL(domain)
	status, reported, err := probeSearchHealthURL(ctx, url)
	if err != nil {
		return Evidence{ExitCode: 1, OutputExcerpt: err.Error()}
	}
	return Evidence{OutputExcerpt: fmt.Sprintf("GET %s -> %d (status %s)", url, status, reported)}
}

// probeSearchHealthURL GETs a search health route and returns the HTTP status
// together with the status the search engine reports.
//
// The payload is required: a search engine answers this route with
// {"status":"green|yellow|red"}, so a proxy error page — which answers 200 with
// HTML — or a foreign JSON API on the same port is not an answer. The colour is
// reported, not judged: a single-node development cluster reports yellow, and a
// cluster the engine itself calls red answers 503 here.
func probeSearchHealthURL(ctx context.Context, url string) (int, string, error) {
	status, body, err := probeHTTP(ctx, url)
	if err != nil {
		return 0, "", fmt.Errorf("GET %s failed: %w", url, err)
	}

	var health struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &health); err != nil {
		return status, "", fmt.Errorf("GET %s -> %d is not a search health payload: %w", url, status, err)
	}
	if health.Status == "" {
		return status, "", fmt.Errorf("GET %s -> %d carries no %q field", url, status, "status")
	}
	if status < 200 || status >= 400 {
		return status, health.Status, fmt.Errorf("GET %s -> %d is not 2xx/3xx (status %s)", url, status, health.Status)
	}
	return status, health.Status, nil
}

// ProbeSearchHealthURLForTest exposes probeSearchHealthURL to the tests under
// tests/, which cannot reach an unexported helper (repo convention: production
// buildThing, test wrapper BuildThingForTest). The route's port is fixed, so the
// payload rule can only be driven against a loopback server this way.
func ProbeSearchHealthURLForTest(ctx context.Context, url string) (int, string, error) {
	return probeSearchHealthURL(ctx, url)
}

// sudoProbe is the seam for "can this run use sudo without a prompt". A nil
// value means the real probe.
var sudoProbe func(ctx context.Context) bool

// sudoWithoutPrompt reports whether `sudo -n true` succeeds. A verify child
// never has a terminal, so a command that needs sudo can only work when sudo
// asks for no password.
func sudoWithoutPrompt(ctx context.Context) bool {
	if sudoProbe != nil {
		return sudoProbe(ctx)
	}
	// A go test binary is the hermetic hook (see execGovard): it must not reach
	// for the machine's sudo.
	if isTestBinary(govardBinary()) {
		return true
	}
	return exec.CommandContext(ctx, "sudo", "-n", "true").Run() == nil
}

// SetSudoProbeForTest replaces the sudo probe; nil restores the real one.
func SetSudoProbeForTest(fn func(ctx context.Context) bool) { sudoProbe = fn }
