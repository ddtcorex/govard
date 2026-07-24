// tests/bootstrap_generic_fresh_install_test.go
package tests

import (
	"errors"
	"testing"

	"govard/internal/engine/bootstrap"
)

// fakeFreshInstallBootstrap is a minimal bootstrap.FrameworkBootstrap fake
// for testing bootstrap.GenericFreshInstall's call sequence, without any
// of Options.Runner's real cmd-container/network plumbing.
type fakeFreshInstallBootstrap struct {
	createErr    error
	installErr   error
	createCalls  int
	installCalls int
}

func (f *fakeFreshInstallBootstrap) Name() string               { return "fake" }
func (f *fakeFreshInstallBootstrap) SupportsFreshInstall() bool { return true }
func (f *fakeFreshInstallBootstrap) SupportsClone() bool        { return true }
func (f *fakeFreshInstallBootstrap) FreshCommands() []string    { return nil }
func (f *fakeFreshInstallBootstrap) Configure(string) error     { return nil }
func (f *fakeFreshInstallBootstrap) PostClone(string) error     { return nil }

func (f *fakeFreshInstallBootstrap) CreateProject(projectDir string) error {
	f.createCalls++
	return f.createErr
}

func (f *fakeFreshInstallBootstrap) Install(projectDir string) error {
	f.installCalls++
	return f.installErr
}

func TestGenericFreshInstallRunsCreateInstallThenConfigureAuto(t *testing.T) {
	fake := &fakeFreshInstallBootstrap{}
	configureAutoCalls := 0
	helpers := bootstrap.CmdHelpers{
		ConfigureAuto: func() error {
			configureAutoCalls++
			return nil
		},
	}

	if err := bootstrap.GenericFreshInstall(fake, "/tmp/project", helpers); err != nil {
		t.Fatalf("GenericFreshInstall() error = %v", err)
	}

	if fake.createCalls != 1 {
		t.Errorf("CreateProject calls = %d, want 1", fake.createCalls)
	}
	if fake.installCalls != 1 {
		t.Errorf("Install calls = %d, want 1", fake.installCalls)
	}
	if configureAutoCalls != 1 {
		t.Errorf("ConfigureAuto calls = %d, want 1", configureAutoCalls)
	}
}

func TestGenericFreshInstallStopsAfterCreateProjectError(t *testing.T) {
	wantErr := errors.New("create failed")
	fake := &fakeFreshInstallBootstrap{createErr: wantErr}
	helpers := bootstrap.CmdHelpers{
		ConfigureAuto: func() error {
			t.Fatal("ConfigureAuto should not be called when CreateProject fails")
			return nil
		},
	}

	err := bootstrap.GenericFreshInstall(fake, "/tmp/project", helpers)
	if !errors.Is(err, wantErr) {
		t.Fatalf("GenericFreshInstall() error = %v, want %v", err, wantErr)
	}
	if fake.installCalls != 0 {
		t.Errorf("Install calls = %d, want 0 (must not run after CreateProject fails)", fake.installCalls)
	}
}

func TestGenericFreshInstallWrapsConfigureAutoError(t *testing.T) {
	fake := &fakeFreshInstallBootstrap{}
	wantErr := errors.New("configure failed")
	helpers := bootstrap.CmdHelpers{
		ConfigureAuto: func() error {
			return wantErr
		},
	}

	err := bootstrap.GenericFreshInstall(fake, "/tmp/project", helpers)
	if !errors.Is(err, wantErr) {
		t.Fatalf("GenericFreshInstall() error = %v, want to wrap %v", err, wantErr)
	}
}
