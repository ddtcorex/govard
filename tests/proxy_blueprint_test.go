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

func TestProxyBlueprintPublishesRabbitMQMgmtPort(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "internal", "blueprints", "files", "proxy.yml"))
	if err != nil {
		t.Fatalf("read proxy blueprint: %v", err)
	}

	if !strings.Contains(string(content), `"127.0.0.1:15672:15672"`) {
		t.Fatal("proxy.yml must publish port 15672 on loopback so project.test:15672 is reachable via 127.0.0.1 only")
	}
}

func TestProxyBlueprintRabbitMQMgmtBindsLoopback(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "internal", "blueprints", "files", "proxy.yml"))
	if err != nil {
		t.Fatalf("read proxy blueprint: %v", err)
	}
	stripped := strings.ReplaceAll(string(content), `"127.0.0.1:15672:15672"`, "")
	if strings.Contains(stripped, "15672:15672") {
		t.Fatal("proxy.yml exposes 15672 on 0.0.0.0")
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

func TestProxyBlueprintPublishesSSHGateway(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "internal", "blueprints", "files", "proxy.yml"))
	if err != nil {
		t.Fatalf("read proxy blueprint: %v", err)
	}
	if !strings.Contains(string(content), `"127.0.0.1:2222:22"`) {
		t.Fatal("proxy.yml must publish the SSH gateway on 127.0.0.1:2222")
	}
	if !strings.Contains(string(content), "govard-proxy-sshd") {
		t.Fatal("proxy.yml must name the govard-proxy-sshd container")
	}
	if !strings.Contains(string(content), "../gateway:/govard-gateway:ro") {
		t.Fatal("proxy.yml must mount the gateway registry read-only into the sshd container")
	}
}
