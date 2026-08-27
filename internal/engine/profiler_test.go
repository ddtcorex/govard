package engine

import "testing"

func TestProfilerLease(t *testing.T) {
	csv, sha, err := ProfilerLease("https://bebe9.test/eveil/livres/livre-sonore.html")
	if err != nil {
		t.Fatalf("ProfilerLease failed: %v", err)
	}
	if csv == "" || sha == "" {
		t.Fatal("csv and sha must not be empty")
	}
}

func TestDbSlowThreshold(t *testing.T) {
	if DbSlowThreshold() != 1 {
		t.Fatal("threshold must be 1s")
	}
}

func TestXdebugGuard(t *testing.T) {
	cfg := Config{}
	cfg.Stack.Features.Xdebug = true
	if !XdebugGuard(cfg) {
		t.Fatal("XdebugGuard must be true when xdebug enabled")
	}
	cfg.Stack.Features.Xdebug = false
	if XdebugGuard(cfg) {
		t.Fatal("XdebugGuard must be false when xdebug disabled")
	}
}
