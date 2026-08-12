//go:build integration
// +build integration

package integration

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"govard/internal/engine"
)

const pinnedBrowserSyncVersion = "2.29.3"

type synchronizedBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *synchronizedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(data)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.String()
}

type liveBrowserSyncProcess struct {
	ctx      context.Context
	cancel   context.CancelFunc
	cmd      *exec.Cmd
	done     chan struct{}
	waitErr  error
	logs     *synchronizedBuffer
	stopOnce sync.Once
}

func TestFrontendSyncPinnedBrowserSyncPreservesMagentoHostAndCookie(t *testing.T) {
	projectDir := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	writeFrontendSyncIntegrationTheme(t, projectDir, "Hyva", "Default")

	config := engine.Config{
		ProjectName: "frontend-sync-live",
		Framework:   "magento2",
		Domain:      "frontend-sync-live.test",
		StoreDomains: engine.StoreDomainMappings{
			"store.frontend-sync-live.test": {Code: "store"},
		},
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Features:   engine.Features{FrontendSync: true},
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}
	if err := engine.RenderBlueprint(projectDir, config); err != nil {
		t.Fatalf("render frontend-sync runtime: %v", err)
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health_check.php" {
			http.NotFound(w, r)
			return
		}
		cookie := "session=abc; Domain=" + r.Host + "; Path=/; Secure; HttpOnly; SameSite=Lax"
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Connection", "close")
		w.Header().Set("X-Upstream-Host", r.Host)
		w.Header().Add("Set-Cookie", cookie)
		_, _ = io.WriteString(w, "magento-health")
	}))
	t.Cleanup(upstream.Close)

	port := reserveFrontendSyncPort(t)
	projectConfig := filepath.Join(projectDir, "app/design/frontend/Hyva/Default/web/tailwind/browser-sync.config.cjs")
	projectSource := `module.exports = {
  proxy: { target: process.env.GOVARD_FRONTEND_SYNC_TARGET, proxyOptions: { changeOrigin: false }, cookies: { stripDomain: false } },
  ghostMode: false,
  socket: { domain: "//'+location.host+'" }
};
`
	if err := os.WriteFile(projectConfig, []byte(projectSource), 0o600); err != nil {
		t.Fatalf("write project BrowserSync config: %v", err)
	}
	wrapperConfig := filepath.Join(t.TempDir(), "browser-sync-live.cjs")
	wrapper := fmt.Sprintf(`
process.env.GOVARD_FRONTEND_SYNC_TARGET = %s;
const config = require(%s);
config.proxy.target = %s;
config.port = %d;
config.online = false;
config.open = false;
config.logLevel = "silent";
module.exports = config;
`, strconv.Quote("http://frontend-sync-live-web-1"), strconv.Quote(projectConfig), strconv.Quote(upstream.URL), port)
	if err := os.WriteFile(wrapperConfig, []byte(wrapper), 0o600); err != nil {
		t.Fatalf("write BrowserSync live wrapper: %v", err)
	}

	process := startPinnedBrowserSync(t, wrapperConfig)
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
	proxyURL := fmt.Sprintf("http://127.0.0.1:%d/health_check.php", port)
	waitForPinnedBrowserSync(t, process, client, proxyURL, config.Domain)

	for _, domain := range []string{config.Domain, "store.frontend-sync-live.test"} {
		response := requestFrontendSync(t, process.ctx, client, proxyURL, domain)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("BrowserSync status for %s = %d, want 200; logs:\n%s", domain, response.StatusCode, process.logs.String())
		}
		if response.Body != "magento-health" {
			t.Fatalf("BrowserSync body for %s = %q, want upstream health response", domain, response.Body)
		}
		if response.UpstreamHost != domain {
			t.Fatalf("upstream Host for %s = %q, want original Host", domain, response.UpstreamHost)
		}
		wantCookie := []string{"session=abc; Domain=" + domain + "; Path=/; Secure; HttpOnly; SameSite=Lax"}
		if !reflect.DeepEqual(response.SetCookies, wantCookie) {
			t.Fatalf("Set-Cookie for %s = %#v, want byte-preserved %#v", domain, response.SetCookies, wantCookie)
		}
	}

	upstream.Close()
	unreachable := requestFrontendSyncAllowError(process.ctx, client, proxyURL, config.Domain)
	if unreachable.Err == nil && unreachable.StatusCode >= 200 && unreachable.StatusCode < 400 {
		t.Fatalf("application health path succeeded with unreachable web upstream: status=%d body=%q", unreachable.StatusCode, unreachable.Body)
	}
}

type frontendSyncHTTPResponse struct {
	StatusCode   int
	Body         string
	UpstreamHost string
	SetCookies   []string
	Err          error
}

func requestFrontendSync(t *testing.T, ctx context.Context, client *http.Client, target, host string) frontendSyncHTTPResponse {
	t.Helper()
	response := requestFrontendSyncAllowError(ctx, client, target, host)
	if response.Err != nil {
		t.Fatalf("request BrowserSync for %s: %v", host, response.Err)
	}
	return response
}

func requestFrontendSyncAllowError(ctx context.Context, client *http.Client, target, host string) frontendSyncHTTPResponse {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return frontendSyncHTTPResponse{Err: err}
	}
	request.Host = host

	response, err := client.Do(request)
	if err != nil {
		return frontendSyncHTTPResponse{Err: err}
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(response.Body)
	return frontendSyncHTTPResponse{
		StatusCode:   response.StatusCode,
		Body:         string(body),
		UpstreamHost: response.Header.Get("X-Upstream-Host"),
		SetCookies:   response.Header.Values("Set-Cookie"),
		Err:          readErr,
	}
}

func reserveFrontendSyncPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve BrowserSync port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release BrowserSync port: %v", err)
	}
	return port
}

func startPinnedBrowserSync(t *testing.T, configPath string) *liveBrowserSyncProcess {
	t.Helper()
	if _, err := exec.LookPath("npx"); err != nil {
		t.Fatalf("live BrowserSync regression requires npx: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	logs := &synchronizedBuffer{}
	command := exec.CommandContext(
		ctx,
		"npx",
		"--yes",
		"--package", "browser-sync@"+pinnedBrowserSyncVersion,
		"browser-sync", "start",
		"--config", configPath,
	)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Stdout = logs
	command.Stderr = logs
	command.WaitDelay = 2 * time.Second
	if err := command.Start(); err != nil {
		cancel()
		t.Fatalf("start pinned BrowserSync %s: %v", pinnedBrowserSyncVersion, err)
	}

	process := &liveBrowserSyncProcess{
		ctx:    ctx,
		cancel: cancel,
		cmd:    command,
		done:   make(chan struct{}),
		logs:   logs,
	}
	go func() {
		process.waitErr = command.Wait()
		close(process.done)
	}()
	t.Cleanup(process.stop)
	return process
}

func (p *liveBrowserSyncProcess) stop() {
	p.stopOnce.Do(func() {
		p.cancel()
		select {
		case <-p.done:
			return
		default:
		}
		if p.cmd.Process != nil {
			_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
		}
		select {
		case <-p.done:
		case <-time.After(3 * time.Second):
		}
	})
}

func waitForPinnedBrowserSync(t *testing.T, process *liveBrowserSyncProcess, client *http.Client, target, host string) {
	t.Helper()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		response := requestFrontendSyncAllowError(process.ctx, client, target, host)
		if response.Err == nil && response.StatusCode == http.StatusOK && response.Body == "magento-health" {
			return
		}

		select {
		case <-process.done:
			t.Fatalf("BrowserSync %s exited before readiness: %v\n%s", pinnedBrowserSyncVersion, process.waitErr, process.logs.String())
		case <-process.ctx.Done():
			t.Fatalf("BrowserSync %s readiness timed out: %v\n%s", pinnedBrowserSyncVersion, process.ctx.Err(), process.logs.String())
		case <-ticker.C:
		}
	}
}
