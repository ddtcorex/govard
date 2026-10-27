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

func TestAdminCreateRemedyCommandNamesTheSharedCredential(t *testing.T) {
	got := magento2.AdminCreateRemedyCommand()
	for _, want := range []string{"admin:user:change-password", "--username admin", "AdminGovard123$"} {
		if !strings.Contains(got, want) {
			t.Fatalf("remedy command %q must contain %q", got, want)
		}
	}
}
