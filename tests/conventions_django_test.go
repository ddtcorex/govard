package tests

import (
	"testing"

	"govard/internal/conventions"
)

func TestDjangoConventionsConstants(t *testing.T) {
	if conventions.FrameworkDjango != "django" {
		t.Errorf("FrameworkDjango = %q, want %q", conventions.FrameworkDjango, "django")
	}
	if conventions.DefaultDjangoDBUser != "django" {
		t.Errorf("DefaultDjangoDBUser = %q, want %q", conventions.DefaultDjangoDBUser, "django")
	}
	if conventions.DefaultDjangoDBPass != "django" {
		t.Errorf("DefaultDjangoDBPass = %q, want %q", conventions.DefaultDjangoDBPass, "django")
	}
	if conventions.DefaultDjangoDBName != "django" {
		t.Errorf("DefaultDjangoDBName = %q, want %q", conventions.DefaultDjangoDBName, "django")
	}
}

func TestPythonWorkDirMatchesDjangoComposeWorkingDir(t *testing.T) {
	if conventions.PythonWorkDir != "/app" {
		t.Errorf("PythonWorkDir = %q, want %q", conventions.PythonWorkDir, "/app")
	}
}
