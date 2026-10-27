package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
	"govard/internal/conventions"
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

// The Portainer password is written into proxy.yml as a bcrypt hash, so the
// plaintext in conventions is not the only artefact to keep correct. This test
// is the binding between the two: change one without the other and it fails,
// with the replacement hash in the failure message.
func TestProxyBlueprintPortainerPasswordMatchesConvention(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "internal", "blueprints", "files", "proxy.yml"))
	if err != nil {
		t.Fatalf("read proxy blueprint: %v", err)
	}

	match := regexp.MustCompile(`--admin-password '([^']+)'`).FindStringSubmatch(string(content))
	if len(match) != 2 {
		t.Fatal("proxy.yml must set --admin-password so Portainer starts with the Govard admin credential")
	}
	// The blueprint is a compose file, so every dollar is doubled; undo that
	// before asking bcrypt about it. An undoubled paste would still match
	// bcrypt here, while compose would read $2a as a variable and hand
	// Portainer a broken password, so the escaping is checked on its own.
	if !strings.Contains(match[1], "$$") {
		t.Fatalf("proxy.yml --admin-password must escape every dollar as $$ for compose, got %q", match[1])
	}
	hash := strings.ReplaceAll(match[1], "$$", "$")

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(conventions.DefaultAdminPassword)); err != nil {
		suggested, genErr := bcrypt.GenerateFromPassword([]byte(conventions.DefaultAdminPassword), 5)
		if genErr != nil {
			t.Fatalf("proxy.yml --admin-password does not match conventions.DefaultAdminPassword: %v", err)
		}
		// bcrypt generates the $2a$ dialect; the blueprint carries $2y$ from
		// PHP, and both verify the same password, so the suggestion keeps the
		// shape of what is already there.
		pasted := strings.Replace(string(suggested), "$2a$", "$2y$", 1)
		t.Fatalf("proxy.yml --admin-password does not match conventions.DefaultAdminPassword (%v); paste this hash into proxy.yml: '%s'",
			err, strings.ReplaceAll(pasted, "$", "$$"))
	}
}
