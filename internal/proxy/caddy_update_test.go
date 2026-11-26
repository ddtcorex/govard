package proxy

import (
	"slices"
	"strings"
	"testing"
)

type recordedCaddyCall struct {
	container string
	stdin     string
	args      []string
}

func caddyArgsContain(args []string, want string) bool {
	return slices.Contains(args, want)
}

func caddyHeaderValue(args []string, name string) (string, bool) {
	prefix := name + ":"
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(arg, prefix)), true
		}
	}
	return "", false
}

func caddyHTTPResponse(etag string, body string) []byte {
	return []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nEtag: " + etag + "\r\n\r\n" + body)
}

// stubCaddyRunner replaces the with-stdin seam and records every call.
func stubCaddyRunner(t *testing.T, fn func(call recordedCaddyCall) ([]byte, error)) *[]recordedCaddyCall {
	t.Helper()
	calls := make([]recordedCaddyCall, 0, 4)
	original := caddyExecRunnerWithStdin
	caddyExecRunnerWithStdin = func(container string, stdin []byte, args ...string) ([]byte, error) {
		call := recordedCaddyCall{container: container, stdin: string(stdin), args: args}
		calls = append(calls, call)
		return fn(call)
	}
	t.Cleanup(func() { caddyExecRunnerWithStdin = original })
	return &calls
}

func TestUpdateCaddyConfigSendsIfMatch(t *testing.T) {
	calls := stubCaddyRunner(t, func(call recordedCaddyCall) ([]byte, error) {
		if caddyArgsContain(call.args, "-X") && caddyArgsContain(call.args, "POST") {
			return []byte("200"), nil
		}
		return caddyHTTPResponse(`"/config/ a"`, `{"apps":{"http":{"servers":{"govard":{}}}}}`), nil
	})

	err := updateCaddyConfig("synthetic-caddy", func(config map[string]interface{}) bool {
		config["marker"] = "yes"
		return true
	})
	if err != nil {
		t.Fatalf("updateCaddyConfig: %v", err)
	}

	var posts []recordedCaddyCall
	for _, call := range *calls {
		if caddyArgsContain(call.args, "POST") {
			posts = append(posts, call)
		}
	}
	if len(posts) != 1 {
		t.Fatalf("got %d POSTs, want 1 (%#v)", len(posts), *calls)
	}
	if !slices.Contains(posts[0].args, caddyAdminAPI+"/config/") {
		t.Fatalf("POST args %#v do not target the /config/ endpoint", posts[0].args)
	}
	etag, ok := caddyHeaderValue(posts[0].args, "If-Match")
	if !ok {
		t.Fatalf("POST args %#v carry no If-Match header", posts[0].args)
	}
	if etag != `"/config/ a"` {
		t.Fatalf("If-Match is %q, want the ETag from the fetch", etag)
	}
	if !caddyArgsContain(posts[0].args, "-d") || !caddyArgsContain(posts[0].args, "@-") {
		t.Fatalf("POST args %#v do not read the payload from stdin", posts[0].args)
	}
	if strings.Contains(strings.Join(posts[0].args, " "), `"marker"`) {
		t.Fatalf("the payload leaked into argv: %#v", posts[0].args)
	}
	if !strings.Contains(posts[0].stdin, `"marker":"yes"`) {
		t.Fatalf("POST stdin %q does not carry the mutated payload", posts[0].stdin)
	}
}

func TestUpdateCaddyConfigRetriesOn412(t *testing.T) {
	gets := 0
	mutations := 0
	calls := stubCaddyRunner(t, func(call recordedCaddyCall) ([]byte, error) {
		isPost := caddyArgsContain(call.args, "POST")
		if !isPost {
			gets++
			if gets == 1 {
				return caddyHTTPResponse(`"/config/ a"`, `{"apps":{"http":{"servers":{"govard":{"generation":"first"}}}}}`), nil
			}
			return caddyHTTPResponse(`"/config/ b"`, `{"apps":{"http":{"servers":{"govard":{"generation":"second"}}}}}`), nil
		}
		if strings.Contains(call.stdin, `"attempt":1`) {
			return []byte("412"), nil
		}
		return []byte("200"), nil
	})

	err := updateCaddyConfig("synthetic-caddy", func(config map[string]interface{}) bool {
		mutations++
		config["attempt"] = mutations
		return true
	})
	if err != nil {
		t.Fatalf("updateCaddyConfig: %v", err)
	}

	posts := 0
	var secondPost recordedCaddyCall
	for _, call := range *calls {
		if caddyArgsContain(call.args, "POST") {
			posts++
			secondPost = call
		}
	}
	if gets != 2 {
		t.Fatalf("got %d fetches, want 2 (the second after the 412)", gets)
	}
	if posts != 2 {
		t.Fatalf("got %d POSTs, want 2", posts)
	}
	etag, _ := caddyHeaderValue(secondPost.args, "If-Match")
	if etag != `"/config/ b"` {
		t.Fatalf("second POST If-Match is %q, want the re-fetched ETag", etag)
	}
	if mutations != 2 {
		t.Fatalf("mutate ran %d times, want it applied again to the re-fetched config", mutations)
	}
	if !strings.Contains(secondPost.stdin, `"generation":"second"`) {
		t.Fatalf("second POST stdin %q was not built from the re-fetched config", secondPost.stdin)
	}
}

func TestUpdateCaddyConfigGivesUpAfterBoundedConflicts(t *testing.T) {
	calls := stubCaddyRunner(t, func(call recordedCaddyCall) ([]byte, error) {
		if caddyArgsContain(call.args, "POST") {
			return []byte("412"), nil
		}
		return caddyHTTPResponse(`"/config/ a"`, `{"apps":{}}`), nil
	})

	err := updateCaddyConfig("synthetic-caddy", func(config map[string]interface{}) bool {
		config["marker"] = "yes"
		return true
	})
	if err == nil {
		t.Fatal("updateCaddyConfig succeeded although every write conflicted, want an error")
	}
	if !strings.Contains(err.Error(), "caddy config changed concurrently") {
		t.Fatalf("error %q does not say the config changed concurrently", err)
	}

	posts := 0
	for _, call := range *calls {
		if caddyArgsContain(call.args, "POST") {
			posts++
		}
	}
	if posts != caddyUpdateAttempts {
		t.Fatalf("got %d POSTs, want the bounded %d", posts, caddyUpdateAttempts)
	}
}

func TestUpdateCaddyConfigSkipsWriteWhenUnchanged(t *testing.T) {
	calls := stubCaddyRunner(t, func(_ recordedCaddyCall) ([]byte, error) {
		return caddyHTTPResponse(`"/config/ a"`, `{"apps":{}}`), nil
	})

	err := updateCaddyConfig("synthetic-caddy", func(map[string]interface{}) bool { return false })
	if err != nil {
		t.Fatalf("updateCaddyConfig: %v", err)
	}
	for _, call := range *calls {
		if caddyArgsContain(call.args, "POST") {
			t.Fatalf("a no-op mutation still wrote the config: %#v", call.args)
		}
	}
}

// A rejected write must fail rather than be retried: only a 412 is a race.
func TestUpdateCaddyConfigFailsOnRejectedWrite(t *testing.T) {
	posts := 0
	stubCaddyRunner(t, func(call recordedCaddyCall) ([]byte, error) {
		if caddyArgsContain(call.args, "POST") {
			posts++
			return []byte("500"), nil
		}
		return caddyHTTPResponse(`"/config/ a"`, `{"apps":{}}`), nil
	})

	err := updateCaddyConfig("synthetic-caddy", func(config map[string]interface{}) bool {
		config["marker"] = "yes"
		return true
	})
	if err == nil {
		t.Fatal("updateCaddyConfig accepted an HTTP 500 reply, want an error")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("error %q does not name the status", err)
	}
	if posts != 1 {
		t.Fatalf("got %d POSTs for a rejected write, want no retry", posts)
	}
}

// Without an ETag the write degrades to last writer wins, deliberately: a proxy
// that returns no ETag must still be able to take a route.
func TestUpdateCaddyConfigWritesWithoutETagWhenTheProxyHasNone(t *testing.T) {
	var post recordedCaddyCall
	stubCaddyRunner(t, func(call recordedCaddyCall) ([]byte, error) {
		if caddyArgsContain(call.args, "POST") {
			post = call
			return []byte("200"), nil
		}
		return []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"apps\":{}}"), nil
	})

	if err := updateCaddyConfig("synthetic-caddy", func(config map[string]interface{}) bool {
		config["marker"] = "yes"
		return true
	}); err != nil {
		t.Fatalf("updateCaddyConfig: %v", err)
	}
	if _, ok := caddyHeaderValue(post.args, "If-Match"); ok {
		t.Fatalf("POST args %#v carry If-Match although the proxy returned no ETag", post.args)
	}
}

// The bounded-conflict error must say the condition once.
func TestUpdateCaddyConfigConflictErrorDoesNotRepeatItself(t *testing.T) {
	stubCaddyRunner(t, func(call recordedCaddyCall) ([]byte, error) {
		if caddyArgsContain(call.args, "POST") {
			return []byte("412"), nil
		}
		return caddyHTTPResponse(`"/config/ a"`, `{"apps":{}}`), nil
	})
	restore := caddyUpdateBackoff
	caddyUpdateBackoff = func(int) {}
	t.Cleanup(func() { caddyUpdateBackoff = restore })

	err := updateCaddyConfig("synthetic-caddy", func(config map[string]interface{}) bool {
		config["marker"] = "yes"
		return true
	})
	if err == nil {
		t.Fatal("updateCaddyConfig succeeded although every write conflicted")
	}
	if strings.Count(err.Error(), "concurrently") != 1 {
		t.Fatalf("error %q repeats the conflict condition", err)
	}
}

// TestRegisterEnvironmentDoesOneReadAndOneWrite pins the batching: three route
// families, one fetch, one write. Three separate calls were three of each, and
// every trip is a docker exec round to the proxy.
func TestRegisterEnvironmentDoesOneReadAndOneWrite(t *testing.T) {
	gets, posts := 0, 0
	var payload string
	stubCaddyRunner(t, func(call recordedCaddyCall) ([]byte, error) {
		if caddyArgsContain(call.args, "POST") {
			posts++
			payload = call.stdin
			return []byte("200"), nil
		}
		gets++
		return caddyHTTPResponse(`"/config/ a"`, `{"apps":{}}`), nil
	})

	err := RegisterEnvironment(EnvironmentRegistration{
		Domains:         []string{"sample-project.test"},
		DomainTarget:    "sample-project-varnish-1:80",
		SearchDomains:   []string{"sample-project.test"},
		SearchTarget:    "sample-project-elasticsearch-1:9200",
		RabbitMQDomains: []string{"sample-project.test"},
		RabbitMQTarget:  "sample-project-rabbitmq-1:15672",
	})
	if err != nil {
		t.Fatalf("RegisterEnvironment: %v", err)
	}
	if gets != 1 || posts != 1 {
		t.Fatalf("%d fetches and %d writes, want one of each", gets, posts)
	}
	for _, want := range []string{"sample-project-varnish-1:80", "sample-project-elasticsearch-1:9200", "sample-project-rabbitmq-1:15672"} {
		if !strings.Contains(payload, want) {
			t.Fatalf("written config %q does not carry %q", payload, want)
		}
	}
}

// A second registration of the same routes must not write at all.
func TestRegisterEnvironmentSkipsTheWriteWhenNothingChanged(t *testing.T) {
	gets, posts := 0, 0
	state := `{"apps":{}}`
	stubCaddyRunner(t, func(call recordedCaddyCall) ([]byte, error) {
		if caddyArgsContain(call.args, "POST") {
			posts++
			state = call.stdin
			return []byte("200"), nil
		}
		gets++
		return caddyHTTPResponse(`"/config/ a"`, state), nil
	})

	registration := EnvironmentRegistration{Domains: []string{"sample-project.test"}, DomainTarget: "sample-project-web-1:80"}
	if err := RegisterEnvironment(registration); err != nil {
		t.Fatalf("first RegisterEnvironment: %v", err)
	}
	posts = 0
	if err := RegisterEnvironment(registration); err != nil {
		t.Fatalf("second RegisterEnvironment: %v", err)
	}
	if posts != 0 {
		t.Fatalf("a repeated registration still wrote %d times", posts)
	}
}
