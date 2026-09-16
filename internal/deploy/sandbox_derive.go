package deploy

import (
	"time"
)

// DerivedFrom records which origin project a sandbox was seeded from and when,
// so later runs can explain drift instead of silently diverging.
type DerivedFrom struct {
	Origin       string `json:"origin"`
	BlueprintRev string `json:"blueprint_rev,omitempty"`
	SeededAt     string `json:"seeded_at"`
}

// NewDerivedFrom stamps a derivation record; empty blueprintRev means the
// caller could not determine one, which the state keeps honestly.
func NewDerivedFrom(origin, blueprintRev string) DerivedFrom {
	return DerivedFrom{Origin: origin, BlueprintRev: blueprintRev, SeededAt: time.Now().UTC().Format(time.RFC3339)}
}
