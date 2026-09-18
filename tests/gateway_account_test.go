package tests

import (
	"context"
	"errors"
	"strings"
	"testing"

	"govard/internal/deploy"
	"govard/internal/gateway"
)

// fakeContainerExec records the argv it was called with and answers with a
// scripted error.
type fakeContainerExec struct {
	calls [][]string
	err   error
}

func (f *fakeContainerExec) ExecInContainer(ctx context.Context, name string, args ...string) (string, error) {
	_ = ctx
	f.calls = append(f.calls, append([]string{name}, args...))
	if f.err != nil {
		return "", f.err
	}
	return "", nil
}

func TestEnsureTargetAccountUsesTheReconcilerUseradd(t *testing.T) {
	fake := &fakeContainerExec{}
	if err := gateway.EnsureTargetAccount(context.Background(), fake, "govard-proxy-sshd", "shop"); err != nil {
		t.Fatalf("EnsureTargetAccount: %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("calls = %d, want exactly one exec", len(fake.calls))
	}
	want := []string{"govard-proxy-sshd", "useradd", "--non-unique", "--uid", "0", "--gid", "0",
		"--no-create-home", "--home-dir", "/nonexistent", "--shell", "/bin/sh", "--", "shop"}
	if strings.Join(fake.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("argv = %q, want %q", fake.calls[0], want)
	}
}

func TestEnsureTargetAccountTreatsAnExistingAccountAsSuccess(t *testing.T) {
	fake := &fakeContainerExec{err: errors.New("useradd: user 'shop' already exists")}
	if err := gateway.EnsureTargetAccount(context.Background(), fake, "govard-proxy-sshd", "shop"); err != nil {
		t.Fatalf("an existing account is the idempotent re-up case, got: %v", err)
	}
}

func TestEnsureTargetAccountReportsOtherFailures(t *testing.T) {
	fake := &fakeContainerExec{err: errors.New("Error: No such container: govard-proxy-sshd")}
	if err := gateway.EnsureTargetAccount(context.Background(), fake, "govard-proxy-sshd", "shop"); err == nil {
		t.Fatal("expected an error when the exec fails")
	}
}

// TestEnsureTargetAccountSurfacesCommandErrorStderr pins that a
// deploy.CommandError-shaped failure keeps its stderr visible through the
// wrapper: the production exec path reports this shape, not a bare
// errors.New, so the diagnostic must survive EnsureTargetAccount.
func TestEnsureTargetAccountSurfacesCommandErrorStderr(t *testing.T) {
	fake := &fakeContainerExec{err: &deploy.CommandError{
		Command:  "docker exec govard-proxy-sshd useradd -- shop",
		ExitCode: 1,
		Stderr:   "useradd: cannot lock /etc/passwd; try again later",
		Err:      errors.New("exit status 1"),
	}}
	err := gateway.EnsureTargetAccount(context.Background(), fake, "govard-proxy-sshd", "shop")
	if err == nil {
		t.Fatal("expected an error when the exec fails")
	}
	if !strings.Contains(err.Error(), "useradd: cannot lock /etc/passwd; try again later") {
		t.Fatalf("error %q does not surface the exec stderr", err.Error())
	}
}
