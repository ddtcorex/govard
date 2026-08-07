package tests

import (
	"testing"

	"govard/internal/conventions"
	"govard/internal/frameworks/django"
)

func TestDjangoConventionsConstants(t *testing.T) {
	if django.DefaultDBUser != "django" {
		t.Errorf("DefaultDBUser = %q, want %q", django.DefaultDBUser, "django")
	}
	if django.DefaultDBPass != "django" {
		t.Errorf("DefaultDBPass = %q, want %q", django.DefaultDBPass, "django")
	}
	if django.DefaultDBName != "django" {
		t.Errorf("DefaultDBName = %q, want %q", django.DefaultDBName, "django")
	}
}

func TestPythonWorkDirMatchesDjangoComposeWorkingDir(t *testing.T) {
	if conventions.PythonWorkDir != "/app" {
		t.Errorf("PythonWorkDir = %q, want %q", conventions.PythonWorkDir, "/app")
	}
}
