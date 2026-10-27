package tests

import (
	"errors"
	"strings"
	"testing"

	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/magento2"
)

func TestRunAdminCreateReportsApplied(t *testing.T) {
	applied := false
	helpers := bootstrap.CmdHelpers{
		IsPHPContainerRunning: func() bool { return true },
		RunToolSilent:         func(string, []string) error { return nil },
	}

	if ok := magento2.RunAdminCreate(bootstrap.Options{AdminApplied: &applied, Domain: "sample.test"}, helpers); !ok {
		t.Fatal("RunAdminCreate must report success when the tool call succeeds")
	}
	if !applied {
		t.Fatal("RunAdminCreate must set AdminApplied when it wrote the credential")
	}
}

func TestRunAdminCreateReportsNotAppliedWhenUserExists(t *testing.T) {
	applied := false
	helpers := bootstrap.CmdHelpers{
		IsPHPContainerRunning: func() bool { return true },
		RunToolSilent:         func(string, []string) error { return errors.New("user already exists") },
	}

	if ok := magento2.RunAdminCreate(bootstrap.Options{AdminApplied: &applied, Domain: "sample.test"}, helpers); ok {
		t.Fatal("RunAdminCreate must not report success when the tool call fails")
	}
	if applied {
		t.Fatal("RunAdminCreate must leave AdminApplied false when the cloned database kept its own admin user")
	}
}

// A cloned Magento database usually already owns the "admin" username, so the
// remedy cannot reuse it and Magento ships no command to change an admin
// password (only admin:user:create and admin:user:unlock exist). The remedy is
// therefore a second admin account with a distinct username, built from the
// option names AdminUserCreateCommand actually defines.
func TestAdminCreateRemedyCommandCreatesASecondAdmin(t *testing.T) {
	got := magento2.AdminCreateRemedyCommand("sample.test")
	for _, want := range []string{
		"admin:user:create",
		"--admin-user=govard",
		"--admin-password='AdminGovard123$'",
		"--admin-email=govard@sample.test",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("remedy command %q must contain %q", got, want)
		}
	}
	if strings.Contains(got, "--username") {
		t.Fatalf("remedy command %q must not use --username: admin:user:create has no such option", got)
	}
	if strings.Contains(got, "change-password") {
		t.Fatalf("remedy command %q must not name change-password: Magento ships no such command", got)
	}
}

func TestAdminCreateRemedyCommandFallsBackWithoutADomain(t *testing.T) {
	if got := magento2.AdminCreateRemedyCommand(""); !strings.Contains(got, "--admin-email=govard@local.test") {
		t.Fatalf("remedy command %q must fall back to a local email when the project has no domain", got)
	}
}

func TestPostClonePropagatesAdminApplied(t *testing.T) {
	applied := false
	helpers := bootstrap.CmdHelpers{
		IsPHPContainerRunning: func() bool { return true },
		RunToolSilent:         func(string, []string) error { return nil },
		RunTool:               func(string, []string) error { return nil },
	}

	if err := magento2.PostClone(bootstrap.Options{AdminApplied: &applied, Domain: "sample.test", AdminCreate: true}, ".", helpers); err != nil {
		t.Fatalf("PostClone returned an error: %v", err)
	}
	if !applied {
		t.Fatal("PostClone must propagate the admin report from RunAdminCreate")
	}
}

func TestPostCloneWithoutAdminCreateLeavesReportFalse(t *testing.T) {
	applied := false
	helpers := bootstrap.CmdHelpers{
		IsPHPContainerRunning: func() bool { return true },
		RunToolSilent:         func(string, []string) error { return nil },
		RunTool:               func(string, []string) error { return nil },
	}

	if err := magento2.PostClone(bootstrap.Options{AdminApplied: &applied, Domain: "sample.test", AdminCreate: false}, ".", helpers); err != nil {
		t.Fatalf("PostClone returned an error: %v", err)
	}
	if applied {
		t.Fatal("--no-admin runs must not report the credential as applied")
	}
}
