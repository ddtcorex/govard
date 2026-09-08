package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProxyBlueprintContainsCaddyResumeFlag(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "internal", "blueprints", "files", "proxy.yml"))
	if err != nil {
		t.Fatalf("read proxy blueprint: %v", err)
	}

	if !strings.Contains(string(content), "--resume") {
		t.Fatal("proxy.yml must contain --resume flag for Caddy to persist config across restarts")
	}
}

func TestProxyBlueprintPublishesSearchPort(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "internal", "blueprints", "files", "proxy.yml"))
	if err != nil {
		t.Fatalf("read proxy blueprint: %v", err)
	}

	if !strings.Contains(string(content), `"9200:9200"`) {
		t.Fatal("proxy.yml must publish port 9200 so project.test:9200 can reach a project's search engine")
	}
}

func TestProxyBlueprintDnsmasqAnswersTestLocally(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "internal", "blueprints", "files", "proxy.yml"))
	if err != nil {
		t.Fatalf("read proxy blueprint: %v", err)
	}

	// dnsmasq -A only synthesizes A records; without --local, AAAA queries for
	// *.test are forwarded upstream (docker DNS -> host stub -> back to dnsmasq)
	// and loop until timeout (~6s per lookup for every dual-stack client).
	if !strings.Contains(string(content), "--local=/test/") {
		t.Fatal("proxy.yml dnsmasq command must contain --local=/test/ so *.test is never forwarded upstream")
	}
}
