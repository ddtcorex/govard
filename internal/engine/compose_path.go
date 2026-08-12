package engine

import (
	"crypto/sha1"
	"govard/internal/conventions"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var composeNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
var projectNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// GovardHomeDir resolves the Govard home directory.
func GovardHomeDir() string {
	return conventions.GetGovardHome()
}

// ComposeFilePath resolves the compose file location for a project.
func ComposeFilePath(projectRoot string, projectName string) string {
	return ComposeFilePathWithProfile(projectRoot, projectName, "")
}

// ComposeFilePathWithProfile resolves the compose file location for a project and profile.
func ComposeFilePathWithProfile(projectRoot string, projectName string, profile string) string {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		if cwd, err := os.Getwd(); err == nil {
			root = cwd
		}
	}
	root = filepath.Clean(root)
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = filepath.Clean(resolved)
	}

	name := sanitizeComposeProjectName(projectName)
	if name == "" {
		name = sanitizeComposeProjectName(inferProjectName(root))
	}
	if name == "" {
		name = "project"
	}

	hashSource := root
	profile = strings.TrimSpace(profile)
	if profile != "" {
		name = name + "-" + profile
		hashSource = root + "|" + profile
	}

	sum := sha1.Sum([]byte(hashSource))
	fileName := name + "-" + strings.ToLower(strings.TrimSpace(hexPrefix(sum[:]))) + ".yml"
	return filepath.Join(GovardHomeDir(), "compose", fileName)
}

// FrontendComposeFilePath resolves the dedicated frontend compose file for a
// project and profile. It intentionally cannot collide with the application
// runtime compose file.
func FrontendComposeFilePath(projectRoot, projectName, profile string) string {
	path := ComposeFilePathWithProfile(projectRoot, projectName+"-frontend", profile)
	return filepath.Join(filepath.Dir(path), "frontend", filepath.Base(path))
}

// EnsureComposePathReady creates the compose directory when missing.
func EnsureComposePathReady(path string) error {
	return os.MkdirAll(filepath.Dir(path), conventions.SecretDirPerm)
}

func sanitizeComposeProjectName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	name = composeNameSanitizer.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-._")
	return strings.ToLower(name)
}

func NormalizeProjectName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	name = strings.ReplaceAll(name, " ", "-")
	name = projectNameSanitizer.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-_")
	return strings.ToLower(name)
}

func hexPrefix(raw []byte) string {
	const maxBytes = 6
	if len(raw) > maxBytes {
		raw = raw[:maxBytes]
	}
	hex := make([]byte, len(raw)*2)
	const digits = "0123456789abcdef"
	for i, b := range raw {
		hex[i*2] = digits[b>>4]
		hex[i*2+1] = digits[b&0x0f]
	}
	return string(hex)
}
