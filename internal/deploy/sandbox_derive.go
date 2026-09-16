package deploy

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// DerivedProjectSuffix is the single suffix a sandbox derived project carries
// in its name, its containers, its volumes and its URL.
const DerivedProjectSuffix = "-sandbox"

// DerivedFrom records which origin project a sandbox was seeded from and when,
// so later runs can explain drift instead of silently diverging.
type DerivedFrom struct {
	Origin       string `json:"origin"`
	BlueprintRev string `json:"blueprint_rev,omitempty"`
	SeededAt     string `json:"seeded_at"`
}

// DerivedProjectName renders the sandbox project name for an origin name,
// sanitized exactly like container names (lowercase, docker-safe).
func DerivedProjectName(origin string) string {
	return sandboxSlug(origin + DerivedProjectSuffix)
}

// SandboxDomain renders the sandbox URL domain for an origin domain by
// inserting the suffix before the first dot: `shop.test` becomes
// `shop-sandbox.test`, which needs no new wildcard DNS.
func SandboxDomain(origin string) (string, error) {
	if !domainRe.MatchString(origin) {
		return "", fmt.Errorf("cannot derive a sandbox domain from %q: want a dotted hostname like shop.test", origin)
	}
	dot := strings.Index(origin, ".")
	return origin[:dot] + DerivedProjectSuffix + origin[dot:], nil
}

var domainRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+$`)

// NewDerivedFrom stamps a derivation record; empty blueprintRev means the
// caller could not determine one, which the state keeps honestly.
func NewDerivedFrom(origin, blueprintRev string) DerivedFrom {
	return DerivedFrom{Origin: origin, BlueprintRev: blueprintRev, SeededAt: time.Now().UTC().Format(time.RFC3339)}
}
