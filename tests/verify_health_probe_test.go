package tests

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/verify"
)

// P2-13 and P4-11 described a probe in their titles and then shelled out to
// `govard tool curl …`. `tool` is a fixed subcommand registry with no curl
// (internal/cmd/frameworks.go), so both items died on "unknown flag" with exit
// 2 in every run. Registering curl would not have fixed P2-13 either: the app
// container cannot resolve the project's own domain — the docker aliases cover
// `mail` and the search containers, and <domain> is mapped only for linked
// projects. P4-11 additionally hard-coded http://localhost:9200, which is the
// container's address inside the compose network, while its title promised the
// project's domain on the host route the docs publish
// (docs/workflows/ssl-and-domains.md).
//
// The items therefore probe in-process, and these tests drive them with no
// project, no container and no executor: against loopback servers, against a
// closed loopback port, and against a reserved unroutable address.

// runItem runs one registry item through its own Run. The two items below do
// their own probing, so they need neither a project root nor a fake executor.
func runItem(t *testing.T, id string, cfg engine.Config) verify.Evidence {
	t.Helper()
	item, ok := findItem(id)
	if !ok {
		t.Fatalf("%s is missing from the registry", id)
	}
	return item.Run(context.Background(), cfg, verify.VerifyOpts{})
}

// shellOutFake answers every govard child with exit 2 — what `govard tool curl
// …` really returns, an unknown flag — and records the argv it saw. A test can
// then tell "the item probed the site itself" from "the item shelled out
// again", instead of reading a green that only means the child was faked.
func shellOutFake(t *testing.T) *[]string {
	t.Helper()
	var seen []string
	verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
		seen = append(seen, strings.Join(args, " "))
		return verify.Evidence{ExitCode: 2, OutputExcerpt: "Error: unknown flag: -k"}, true
	})
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })
	return &seen
}

// fakeProbeHTTP makes the host probes hermetic for the suites that drive whole
// phases end-to-end (verify_guard_test.go's installGuardProbe and
// verify_all_phases_test.go's runVerifyAllPhases, as well as the pin below):
// P2-13 and P4-11 do their own HTTP, which the exec fake cannot intercept — a
// `--project <tempdir>` run derives a domain from the directory name
// (…/001 → 001.test) and would otherwise dial it for real.
func fakeProbeHTTP(t *testing.T) {
	t.Helper()
	verify.SetProbeHTTPFakeForTest(func(context.Context, string) (int, []byte, error) {
		return http.StatusOK, []byte(`{"status":"green"}`), nil
	})
	t.Cleanup(func() { verify.SetProbeHTTPFakeForTest(nil) })
}

// closedLoopbackAddr claims a free loopback port and releases it, so a probe
// against it is refused by the kernel and nothing else answers.
func closedLoopbackAddr(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	addr := strings.TrimPrefix(server.URL, "http://")
	server.Close()
	return addr
}

// The probe fake is what keeps a whole-phase test hermetic, so it must actually
// intercept. A reserved, unroutable address makes a real probe fail instantly,
// so a green row here can only have come from the fake.
func TestProbeFakeInterceptsTheProbe(t *testing.T) {
	fakeProbeHTTP(t)

	ev := runItem(t, "P2-13", engine.Config{Domain: "255.255.255.255"})

	if ev.ExitCode != 0 || ev.Skipped {
		t.Fatalf("P2-13 with the probe faked: exit=%d skipped=%v evidence=%q", ev.ExitCode, ev.Skipped, ev.OutputExcerpt)
	}
	if !strings.Contains(ev.OutputExcerpt, "scheme https") {
		t.Fatalf("P2-13 evidence does not name the scheme the fake answered on: %q", ev.OutputExcerpt)
	}
}

// A project that declares no domain gets a skip, not a guess: P2-13 used to
// substitute localhost, which probed whatever the host answered on :80/:443 and
// reported it as the project's site (design §4.2: the item says what is
// missing, and the row stays visible as a skip).
func TestHealthProbesSkipWithoutADomain(t *testing.T) {
	for _, id := range []string{"P2-13", "P4-11"} {
		ev := runItem(t, id, engine.Config{})
		if !ev.Skipped {
			t.Fatalf("%s with no domain: skipped=%v, evidence=%q; want a skip naming the missing domain", id, ev.Skipped, ev.OutputExcerpt)
		}
		if !strings.Contains(ev.SkipReason, "domain") {
			t.Fatalf("%s skip reason = %q, want it to name the missing domain", id, ev.SkipReason)
		}
	}
}

func TestP213PassesWhenTheSiteAnswers(t *testing.T) {
	spawned := shellOutFake(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Plain HTTP on a loopback port: the https attempt cannot complete a
	// handshake here (net/http reports a scheme mismatch), which is the one
	// case that licenses the http:// fallback.
	ev := runItem(t, "P2-13", engine.Config{Domain: strings.TrimPrefix(server.URL, "http://")})

	if len(*spawned) != 0 {
		t.Fatalf("P2-13 spawned a govard child (%v) instead of probing the site", *spawned)
	}
	if ev.ExitCode != 0 {
		t.Fatalf("P2-13 exit = %d against a site answering 200: %q", ev.ExitCode, ev.OutputExcerpt)
	}
	if !strings.Contains(ev.OutputExcerpt, "scheme http") {
		t.Fatalf("P2-13 evidence does not name the scheme that answered: %q", ev.OutputExcerpt)
	}
}

func TestP213FailsWhenNothingAnswers(t *testing.T) {
	spawned := shellOutFake(t)
	addr := closedLoopbackAddr(t)

	ev := runItem(t, "P2-13", engine.Config{Domain: addr})

	if len(*spawned) != 0 {
		t.Fatalf("P2-13 spawned a govard child (%v) instead of probing the site", *spawned)
	}
	if ev.ExitCode == 0 {
		t.Fatalf("P2-13 passed with nothing listening on %s: %q", addr, ev.OutputExcerpt)
	}
	// The verdict must be about the site the item claims to probe, not about a
	// child process that failed for its own reasons.
	if !strings.Contains(ev.OutputExcerpt, "https://"+addr+"/") {
		t.Fatalf("P2-13 evidence does not name the url it probed: %q", ev.OutputExcerpt)
	}
	// A refused connection is not a TLS handshake failure, so it must not
	// license the http:// retry: retrying here would report a green for a
	// domain whose certificate setup was never exercised.
	if strings.Contains(ev.OutputExcerpt, "http://"+addr+"/") {
		t.Fatalf("P2-13 retried over http after a connection error: %q", ev.OutputExcerpt)
	}
}

func TestP411UsesTheProjectDomainNotLocalhost(t *testing.T) {
	argvs := captureItemArgvs(t, engine.Config{Framework: "magento2"}, verify.VerifyOpts{
		ProjectRoot: t.TempDir(),
	})

	if got := argvs["P4-11"]; len(got) != 0 {
		t.Fatalf("P4-11 invoked %v; the item must probe the host route itself", got)
	}
	if got := argvs["P2-13"]; len(got) != 0 {
		t.Fatalf("P2-13 invoked %v; the item must probe the site itself", got)
	}
	// The class, not just this row: no item may send curl at the
	// container-local search port, which nothing on the host answers.
	for id, argv := range argvs {
		if strings.Contains(strings.Join(argv, " "), "localhost:9200") {
			t.Fatalf("%s still targets the container-local search port: %v", id, argv)
		}
	}

	// And the route the item must build itself. A reserved, unroutable address
	// answers nothing and fails instantly, so the evidence names the route the
	// item probed without leaving the process: cfg.Domain on the documented
	// host port, with the path the docs publish.
	ev := runItem(t, "P4-11", engine.Config{Domain: "255.255.255.255"})
	if !strings.Contains(ev.OutputExcerpt, "http://255.255.255.255:9200/_cluster/health") {
		t.Fatalf("P4-11 evidence does not name the route it probed: %q", ev.OutputExcerpt)
	}
	if strings.Contains(ev.OutputExcerpt, "localhost:9200") {
		t.Fatalf("P4-11 still targets the container-local search port: %q", ev.OutputExcerpt)
	}
}

// P2-13 deliberately probes https:// first: the local CA is only trusted after
// `govard doctor trust`, and weakening verification to make the item green is
// the defect this test exists to catch. httptest's TLS server presents a
// certificate no system pool signs, so a verifying client cannot accept it —
// and because the item then falls back to plain HTTP on a TLS-only port, the
// row is red for the reason an operator has to fix.
func TestP213KeepsTLSVerificationEnabled(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ev := runItem(t, "P2-13", engine.Config{Domain: strings.TrimPrefix(server.URL, "https://")})

	if ev.ExitCode == 0 {
		t.Fatalf("P2-13 accepted an untrusted certificate as an answer: %q", ev.OutputExcerpt)
	}
}

// P4-11 must not accept "something answered". A proxy error page answers 200
// with HTML, and a foreign JSON API on the same port answers JSON without a
// status, so the payload — the field a search engine reports its health in — is
// what makes the row mean "the host route reaches the search engine".
//
// The route's port is fixed by the documented host route, so the payload rule
// is driven through the probe itself (the item is a one-line composition of
// searchHealthURL and this function) rather than through a container.
func TestP411RequiresASearchHealthPayload(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body       string
		wantGreen  bool
		wantReport string
	}{
		{name: "health payload", status: 200, body: `{"cluster_name":"govard","status":"green"}`, wantGreen: true, wantReport: "green"},
		{name: "yellow is what a single-node cluster reports", status: 200, body: `{"status":"yellow"}`, wantGreen: true, wantReport: "yellow"},
		{name: "proxy error page", status: 200, body: `<html><body>502 Bad Gateway</body></html>`},
		{name: "json without a status field", status: 200, body: `{"cluster_name":"govard"}`},
		{name: "status is not a string", status: 200, body: `{"status":404}`},
		{name: "red cluster answers 503", status: 503, body: `{"status":"red"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()

			status, reported, err := verify.ProbeSearchHealthURLForTest(context.Background(), server.URL)
			if status != tc.status {
				t.Fatalf("probe reported http status %d, want %d", status, tc.status)
			}
			if tc.wantGreen {
				if err != nil {
					t.Fatalf("probe rejected a health payload %q: %v", tc.body, err)
				}
				if reported != tc.wantReport {
					t.Fatalf("probe reported status %q, want %q", reported, tc.wantReport)
				}
				return
			}
			if err == nil {
				t.Fatalf("probe accepted %q (http %d) as a search health payload (reported %q)", tc.body, tc.status, reported)
			}
			if !strings.Contains(err.Error(), server.URL) {
				t.Fatalf("probe error does not name the route it probed: %v", err)
			}
		})
	}
}
