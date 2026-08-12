//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"govard/internal/proxy"

	"github.com/gorilla/websocket"
)

func TestFrontendLumaLiveEndpointAndHTMLInjection(t *testing.T) {
	SkipIfNoDocker(t)
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required for the Luma injection integration test")
	}
	if err := exec.Command("docker", "image", "inspect", "caddy:latest").Run(); err != nil {
		t.Skip("the caddy:latest image is not available locally")
	}

	application := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/page":
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(writer, "<!doctype html><html><body>synthetic page</body></html>")
		case "/api":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"content":"</body>"}`)
		case "/__govard_frontend_health":
			writer.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(writer, "application health response")
		default:
			http.NotFound(writer, request)
		}
	}))
	defer application.Close()

	liveReload := startSyntheticGruntLiveReload(t)

	injectorPort := reserveFrontendTestPort(t)
	injectorContext, stopInjector := context.WithCancel(context.Background())
	defer stopInjector()
	injectorScript := filepath.Join("..", "..", "internal", "frameworks", "magento2", "blueprint", "support", "frontend-inject.mjs")
	injector := exec.CommandContext(injectorContext, "node", injectorScript)
	injector.Env = append(os.Environ(),
		"GOVARD_FRONTEND_INJECT_UPSTREAM="+application.URL,
		fmt.Sprintf("GOVARD_FRONTEND_INJECT_PORT=%d", injectorPort),
		`GOVARD_FRONTEND_INJECT_SCRIPT_HTML=<script src="/livereload/livereload.js?snipver=1&port=443&path=livereload/livereload"></script>`,
	)
	injectorOutput := &bytes.Buffer{}
	injector.Stdout = injectorOutput
	injector.Stderr = injectorOutput
	if err := injector.Start(); err != nil {
		t.Fatalf("start Luma injector: %v", err)
	}
	defer func() {
		stopInjector()
		_ = injector.Wait()
	}()

	client := &http.Client{Timeout: 2 * time.Second}
	waitForFrontendTestHTTP(t, client, fmt.Sprintf("http://127.0.0.1:%d/__govard_frontend_health", injectorPort), "localhost", injectorOutput)
	runFrontendLumaLiveAssertions(t, application, liveReload, injectorPort)
}

type syntheticGruntLiveReload struct {
	address   string
	clientDir string
	process   *liveFrontendProcess
}

type liveFrontendLogBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (buffer *liveFrontendLogBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.Buffer.Write(data)
}

func (buffer *liveFrontendLogBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.Buffer.String()
}

type liveFrontendProcess struct {
	cancel context.CancelFunc
	cmd    *exec.Cmd
	done   chan struct{}
	logs   *liveFrontendLogBuffer
	once   sync.Once
}

func startLiveFrontendProcess(t *testing.T, dir, name string, args ...string) *liveFrontendProcess {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	logs := &liveFrontendLogBuffer{}
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Stdout = logs
	command.Stderr = logs
	command.WaitDelay = 2 * time.Second
	if err := command.Start(); err != nil {
		cancel()
		t.Fatalf("start %s: %v", name, err)
	}
	process := &liveFrontendProcess{cancel: cancel, cmd: command, done: make(chan struct{}), logs: logs}
	go func() {
		_ = command.Wait()
		close(process.done)
	}()
	t.Cleanup(process.stop)
	return process
}

func (process *liveFrontendProcess) stop() {
	process.once.Do(func() {
		process.cancel()
		select {
		case <-process.done:
			return
		default:
		}
		if process.cmd.Process != nil {
			_ = syscall.Kill(-process.cmd.Process.Pid, syscall.SIGKILL)
		}
		select {
		case <-process.done:
		case <-time.After(3 * time.Second):
		}
	})
}

func startSyntheticGruntLiveReload(t *testing.T) syntheticGruntLiveReload {
	t.Helper()
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm is required for the synthetic Grunt/LiveReload integration test")
	}
	root := t.TempDir()
	port := reserveFrontendTestPort(t)
	packageJSON := `{"private":true,"devDependencies":{"grunt":"1.6.1","grunt-contrib-watch":"1.1.0"}}`
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(packageJSON), 0o644); err != nil {
		t.Fatalf("write synthetic Grunt package.json: %v", err)
	}
	gruntfile := fmt.Sprintf(`module.exports = function (grunt) {
  grunt.initConfig({watch:{options:{livereload:%d},fixture:{files:["fixture.txt"]}}});
  grunt.loadNpmTasks("grunt-contrib-watch");
};
`, port)
	if err := os.WriteFile(filepath.Join(root, "Gruntfile.js"), []byte(gruntfile), 0o644); err != nil {
		t.Fatalf("write synthetic Gruntfile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "fixture.txt"), []byte("synthetic\n"), 0o644); err != nil {
		t.Fatalf("write synthetic watched file: %v", err)
	}
	install := exec.Command("npm", "install", "--ignore-scripts", "--no-audit", "--no-fund")
	install.Dir = root
	if output, err := install.CombinedOutput(); err != nil {
		t.Fatalf("install synthetic Grunt/LiveReload fixture: %v\n%s", err, output)
	}
	process := startLiveFrontendProcess(t, root, "npx", "grunt", "watch", "--no-color")
	client := &http.Client{Timeout: 2 * time.Second}
	address := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForFrontendTestHTTP(t, client, address+"/livereload.js?snipver=1", "localhost", process.logs)
	return syntheticGruntLiveReload{address: address, clientDir: root, process: process}
}

func runFrontendLumaLiveAssertions(t *testing.T, application *httptest.Server, liveReload syntheticGruntLiveReload, injectorPort int) {
	t.Helper()
	caddyPort := reserveFrontendTestPort(t)
	liveReloadAddress := strings.TrimPrefix(liveReload.address, "http://")
	config := map[string]interface{}{
		"admin": map[string]interface{}{"disabled": true},
		"apps": map[string]interface{}{
			"http": map[string]interface{}{
				"servers": map[string]interface{}{
					"srv0": map[string]interface{}{
						"listen":          []interface{}{fmt.Sprintf(":%d", caddyPort)},
						"automatic_https": map[string]interface{}{"disable": true},
						"routes":          []interface{}{},
					},
				},
			},
		},
	}
	const projectDomain = "synthetic-store.test"
	_ = proxy.UpsertDomainRouteForTest(config, projectDomain, strings.TrimPrefix(application.URL, "http://"))
	_ = proxy.UpsertFrontendRegistrationForTest(config, proxy.FrontendRegistration{
		ProjectName: "synthetic-store",
		Domains:     []string{projectDomain},
		Endpoint: proxy.FrontendEndpoint{
			Path:        "/livereload/*",
			Target:      liveReloadAddress,
			StripPrefix: "/livereload",
		},
		HTMLInjectionTarget: fmt.Sprintf("127.0.0.1:%d", injectorPort),
	})
	payload, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal synthetic Caddy config: %v", err)
	}
	configPath := filepath.Join(t.TempDir(), "caddy.json")
	if err := os.WriteFile(configPath, payload, 0o644); err != nil {
		t.Fatalf("write synthetic Caddy config: %v", err)
	}

	caddyContext, stopCaddy := context.WithCancel(context.Background())
	defer stopCaddy()
	caddy := exec.CommandContext(caddyContext, "docker", "run", "--rm", "--network", "host", "-v", configPath+":/etc/caddy/caddy.json:ro", "caddy:latest", "caddy", "run", "--config", "/etc/caddy/caddy.json")
	caddyOutput := &strings.Builder{}
	caddy.Stdout = caddyOutput
	caddy.Stderr = caddyOutput
	if err := caddy.Start(); err != nil {
		t.Fatalf("start synthetic Caddy: %v", err)
	}
	defer func() {
		stopCaddy()
		_ = caddy.Wait()
	}()

	client := &http.Client{Timeout: 2 * time.Second}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", caddyPort)
	waitForFrontendTestHTTP(t, client, baseURL+"/page", projectDomain, caddyOutput)

	page := frontendTestGET(t, client, baseURL+"/page", projectDomain)
	wantScript := `<script src="/livereload/livereload.js?snipver=1&port=443&path=livereload/livereload"></script>`
	if !strings.Contains(page, wantScript+"</body>") || strings.Count(page, wantScript) != 1 {
		t.Fatalf("injected page = %q", page)
	}
	api := frontendTestGET(t, client, baseURL+"/api", projectDomain)
	if api != `{"content":"</body>"}` {
		t.Fatalf("non-HTML response changed: %q", api)
	}
	clientSource := frontendTestGET(t, client, baseURL+"/livereload/livereload.js?snipver=1&port=443&path=livereload/livereload", projectDomain)
	if !strings.Contains(clientSource, "LiveReload") {
		t.Fatalf("LiveReload endpoint did not serve the actual standard client: %q", clientSource[:min(len(clientSource), 200)])
	}
	assertStandardLiveReloadClientOptions(t, liveReload.clientDir, "https://"+projectDomain+strings.TrimPrefix(wantScript, `<script src="`))
	publicHealthPath := frontendTestGET(t, client, baseURL+"/__govard_frontend_health", projectDomain)
	if publicHealthPath != "application health response" {
		t.Fatalf("public application health path was shadowed: %q", publicHealthPath)
	}
	websocketDialer := *websocket.DefaultDialer
	websocketDialer.NetDialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", caddyPort))
	}
	connection, _, err := websocketDialer.Dial("ws://"+projectDomain+fmt.Sprintf(":%d/livereload/livereload", caddyPort), nil)
	if err != nil {
		t.Fatalf("connect to proxied tiny-lr websocket: %v", err)
	}
	defer connection.Close()
	hello := `{"command":"hello","protocols":["http://livereload.com/protocols/official-7"],"ver":"4.0.0"}`
	if err := connection.WriteMessage(websocket.TextMessage, []byte(hello)); err != nil {
		t.Fatalf("send tiny-lr handshake: %v", err)
	}
	_, message, err := connection.ReadMessage()
	if err != nil {
		t.Fatalf("read proxied tiny-lr websocket: %v", err)
	}
	if !strings.Contains(string(message), `"command":"hello"`) {
		t.Fatalf("proxied tiny-lr websocket handshake = %q", message)
	}
}

func assertStandardLiveReloadClientOptions(t *testing.T, fixtureDir, rawScript string) {
	t.Helper()
	scriptURL := strings.TrimSuffix(rawScript, `"></script>`)
	evaluator := `const {Options}=require("livereload-js/lib/options");
const src=process.argv[1];
const element={src,getAttribute:()=>src};
const options=Options.extract({getElementsByTagName:()=>[element]});
process.stdout.write(JSON.stringify({https:options.https,host:options.host,port:options.port,path:options.path,snipver:options.snipver}));`
	command := exec.Command("node", "-e", evaluator, scriptURL)
	command.Dir = fixtureDir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("evaluate injected URL with fixture's standard LiveReload client: %v\n%s", err, output)
	}
	var options struct {
		HTTPS   bool        `json:"https"`
		Host    string      `json:"host"`
		Port    interface{} `json:"port"`
		Path    string      `json:"path"`
		Snipver interface{} `json:"snipver"`
	}
	if err := json.Unmarshal(output, &options); err != nil {
		t.Fatalf("decode standard LiveReload options %q: %v", output, err)
	}
	if !options.HTTPS || options.Host != "synthetic-store.test" || fmt.Sprint(options.Port) != "443" || options.Path != "livereload/livereload" || fmt.Sprint(options.Snipver) != "1" {
		t.Fatalf("standard LiveReload options = %#v, want TLS host, port 443, and public websocket path", options)
	}
}

func reserveFrontendTestPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve frontend integration port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitForFrontendTestHTTP(t *testing.T, client *http.Client, url, host string, diagnostics fmt.Stringer) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		request, _ := http.NewRequest(http.MethodGet, url, nil)
		request.Host = host
		if response, err := client.Do(request); err == nil {
			_ = response.Body.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s\nprocess output:\n%s", url, diagnostics.String())
}

func frontendTestGET(t *testing.T, client *http.Client, url, host string) string {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build GET %s: %v", url, err)
	}
	request.Host = host
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read GET %s: %v", url, err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d: %s", url, response.StatusCode, body)
	}
	return string(body)
}
