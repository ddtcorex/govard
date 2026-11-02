package tests

import (
	"testing"

	"govard/internal/try"
)

func TestResolveOriginServesTheProjectDomainOverHTTPS(t *testing.T) {
	o := try.ResolveOrigin("shop.test", func(string) bool { return true }, func(string) bool { return true })

	if o.URL != "https://shop.test" {
		t.Fatalf("URL = %q, want https://shop.test", o.URL)
	}
	if o.HostHeader != "shop.test" {
		t.Fatalf("HostHeader = %q, want shop.test", o.HostHeader)
	}
	if !o.TLS || !o.Trusted || !o.Resolvable {
		t.Fatalf("origin = %+v, want TLS, trusted and resolvable", o)
	}
}

func TestResolveOriginKeepsHTTPSWhenTheCAIsNotTrusted(t *testing.T) {
	o := try.ResolveOrigin("shop.test", func(string) bool { return true }, func(string) bool { return false })

	if o.URL != "https://shop.test" {
		t.Fatalf("URL = %q, want the served origin https://shop.test", o.URL)
	}
	if o.Trusted {
		t.Fatalf("Trusted = true, want false")
	}
	if !o.Resolvable {
		t.Fatalf("Resolvable = false, want true")
	}
}

func TestResolveOriginReportsAnUnresolvableDomain(t *testing.T) {
	o := try.ResolveOrigin("shop.test", func(string) bool { return false }, func(string) bool { return true })

	if o.Resolvable {
		t.Fatalf("Resolvable = true, want false")
	}
	if o.URL != "https://shop.test" {
		t.Fatalf("URL = %q, want https://shop.test", o.URL)
	}
}

func TestResolveOriginTreatsNilChecksAsUnavailable(t *testing.T) {
	o := try.ResolveOrigin("shop.test", nil, nil)

	if o.Resolvable || o.Trusted {
		t.Fatalf("origin = %+v, want neither resolvable nor trusted", o)
	}
}
