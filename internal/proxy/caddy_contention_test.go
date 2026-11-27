package proxy

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"
)

// fakeCaddy is a minimal in-memory admin API: a document with a version that is
// its ETag, and a POST that answers 412 when If-Match is stale. The fetch takes a
// few milliseconds, which is the window a real `docker exec` leaves open.
type fakeCaddy struct {
	mu      sync.Mutex
	version int
	routes  []interface{}
}

func (f *fakeCaddy) handle(call recordedCaddyCall) ([]byte, error) {
	if caddyArgsContain(call.args, "POST") {
		f.mu.Lock()
		defer f.mu.Unlock()
		match, _ := caddyHeaderValue(call.args, "If-Match")
		if match != fmt.Sprintf(`"/config/ v%d"`, f.version) {
			return []byte("412"), nil
		}
		var posted struct {
			Routes []interface{} `json:"routes"`
		}
		if err := json.Unmarshal([]byte(call.stdin), &posted); err != nil {
			return nil, err
		}
		f.routes = posted.Routes
		f.version++
		return []byte("200"), nil
	}

	f.mu.Lock()
	version := f.version
	body, _ := json.Marshal(map[string]interface{}{"routes": f.routes, "apps": map[string]interface{}{}})
	f.mu.Unlock()
	time.Sleep(3 * time.Millisecond)
	return caddyHTTPResponse(fmt.Sprintf(`"/config/ v%d"`, version), string(body)), nil
}

// Several projects started at once all read the same document and race to write
// it back; each retry lets exactly one of the remaining writers through, so the
// number of attempts is the number of simultaneous registrations that succeed.
// Eight is a team starting its stacks together, not a stress test.
func TestUpdateCaddyConfigSurvivesEightSimultaneousWriters(t *testing.T) {
	const writers = 8
	fake := &fakeCaddy{}
	// Not stubCaddyRunner: it records calls in an unguarded slice, which these
	// concurrent writers would race on.
	originalRunner := caddyExecRunnerWithStdin
	caddyExecRunnerWithStdin = func(container string, stdin []byte, args ...string) ([]byte, error) {
		return fake.handle(recordedCaddyCall{container: container, stdin: string(stdin), args: args})
	}
	t.Cleanup(func() { caddyExecRunnerWithStdin = originalRunner })
	restore := caddyUpdateBackoff
	caddyUpdateBackoff = func(int) {} // no spreading: the adversarial lockstep case
	t.Cleanup(func() { caddyUpdateBackoff = restore })

	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = updateCaddyConfig("synthetic-caddy", func(config map[string]interface{}) bool {
				routes, _ := config["routes"].([]interface{})
				config["routes"] = append(routes, "project-"+strconv.Itoa(i))
				return true
			})
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("writer %d: %v", i, err)
		}
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.routes) != writers {
		t.Fatalf("%d of %d routes were registered: %v", len(fake.routes), writers, fake.routes)
	}
}
