package tests

import (
	"testing"

	"govard/internal/desktop"
)

func TestGlobalServicesRegistersSSHGateway(t *testing.T) {
	compose, container, openable, ok := desktop.GlobalServiceSpecForTest("sshd")
	if !ok {
		t.Fatal("expected a \"sshd\" entry in the desktop global services list")
	}
	if compose != "sshd" {
		t.Fatalf("compose service = %q, want \"sshd\"", compose)
	}
	if container != "govard-proxy-sshd" {
		t.Fatalf("container name = %q, want \"govard-proxy-sshd\"", container)
	}
	if openable {
		t.Fatal("the SSH gateway has no web interface and must not be Openable")
	}
}

func TestGlobalServicesSpecLookupNormalizesID(t *testing.T) {
	compose, container, openable, ok := desktop.GlobalServiceSpecForTest(" SSHD ")
	if !ok {
		t.Fatal("expected an oddly-cased/padded \" SSHD \" to resolve to the sshd entry")
	}
	if compose != "sshd" {
		t.Fatalf("compose service = %q, want \"sshd\"", compose)
	}
	if container != "govard-proxy-sshd" {
		t.Fatalf("container name = %q, want \"govard-proxy-sshd\"", container)
	}
	if openable {
		t.Fatal("the SSH gateway has no web interface and must not be Openable")
	}
}
