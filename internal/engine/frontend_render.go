package engine

import (
	"fmt"
	"io/fs"
)

// ResolveBlueprintsFS resolves the blueprint filesystem for a project
// configuration. Framework packages use it when rendering a dedicated
// Compose artifact outside the application blueprint lifecycle.
func ResolveBlueprintsFS(root string, config Config) (fs.FS, error) {
	blueprintsFS, err := resolveBlueprintsDirForConfig(root, config)
	if err != nil {
		return nil, fmt.Errorf("resolve blueprints directory: %w", err)
	}
	return blueprintsFS, nil
}

// RenderBlueprintTemplate renders one blueprint template with the registered
// template function map. The input data is intentionally framework-owned.
func RenderBlueprintTemplate(blueprintsFS fs.FS, templatePath string, data any) (string, error) {
	return renderTemplateFS(blueprintsFS, templatePath, data)
}

// WriteRenderedCompose writes an already-merged Compose document to path.
func WriteRenderedCompose(path string, document map[string]interface{}) error {
	if err := EnsureComposePathReady(path); err != nil {
		return fmt.Errorf("prepare compose output path: %w", err)
	}
	return writeRenderedCompose(path, document)
}
