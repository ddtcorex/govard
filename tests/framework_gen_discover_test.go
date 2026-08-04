package tests

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"govard/internal/frameworks/gen/generator"
)

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func TestDiscoverFrameworkDirsFindsPackagesWithDefinition(t *testing.T) {
	root := t.TempDir()

	writeTestFile(t, filepath.Join(root, "cakephp", "cakephp.go"), `package cakephp

import "govard/internal/frameworks/types"

func Definition() types.FrameworkDefinition {
	return types.FrameworkDefinition{Name: "cakephp"}
}
`)
	writeTestFile(t, filepath.Join(root, "django", "django.go"), `package django

import "govard/internal/frameworks/types"

func Definition() types.FrameworkDefinition {
	return types.FrameworkDefinition{Name: "django"}
}
`)
	// Shared support packages - must be excluded even though they have .go files.
	writeTestFile(t, filepath.Join(root, "types", "definition.go"), `package types

type FrameworkDefinition struct{ Name string }
`)
	writeTestFile(t, filepath.Join(root, "gen", "main.go"), `package main

func main() {}
`)
	writeTestFile(t, filepath.Join(root, "shared", "dotenv.go"), `package shared

func Parse() {}
`)

	got, err := generator.DiscoverFrameworkDirs(root)
	if err != nil {
		t.Fatalf("DiscoverFrameworkDirs() error = %v", err)
	}

	want := []string{"cakephp", "django"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DiscoverFrameworkDirs() = %v, want %v", got, want)
	}
}

func TestDiscoverFrameworkDirsRejectsDirectoryWithoutDefinition(t *testing.T) {
	root := t.TempDir()

	writeTestFile(t, filepath.Join(root, "broken", "broken.go"), `package broken

func Setup() {}
`)

	_, err := generator.DiscoverFrameworkDirs(root)
	if err == nil {
		t.Fatal("expected an error for a directory with no Definition() func, got nil")
	}
}

func TestDiscoverFrameworkDirsIgnoresTestFiles(t *testing.T) {
	root := t.TempDir()

	// A _test.go file with a Definition-like name must not count - only
	// non-test .go files establish that a directory is a real framework
	// package.
	writeTestFile(t, filepath.Join(root, "onlytests", "onlytests_test.go"), `package onlytests

func Definition() int { return 0 }
`)

	_, err := generator.DiscoverFrameworkDirs(root)
	if err == nil {
		t.Fatal("expected an error - onlytests has no Definition() in a non-test file")
	}
}
