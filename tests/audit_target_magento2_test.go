package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"govard/internal/frameworks/magento2"
	"govard/internal/frameworks/types"
)

func TestResolveMagentoAuditTarget(t *testing.T) {
	root := makeMagentoAuditProject(t, "magento/framework")
	mageOSRoot := makeMagentoAuditProject(t, "mage-os/project-community-edition")
	nestedModule := makeMagentoAuditModule(t, root, "app/code/Example/Nested")
	standaloneModule := makeStandaloneMagentoAuditModule(t)
	declaredModule := makeMagentoAuditDeclaredModule(t, root, "app/code/Example/Declared")
	dualSignalModule := makeMagentoAuditModule(t, root, "app/code/Example/Dual")
	writeAuditModuleDeclaration(t, dualSignalModule, "Example_Dual")
	standaloneDeclaredModule := makeMagentoAuditDeclaredModule(t, t.TempDir(), "Example/Standalone")
	// Mirror a real in-app module that also ships registration.php next to module.xml.
	if err := os.WriteFile(filepath.Join(standaloneDeclaredModule, "registration.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatalf("write module registration: %v", err)
	}
	namespaceDirectory := filepath.Dir(nestedModule)
	// Composer package that keeps its module code under src/, as several
	// distributed modules do: composer.json at the package root, etc/module.xml
	// one level down.
	sourceLayoutPackage := makeStandaloneMagentoAuditModule(t)
	sourceLayoutModule := makeMagentoAuditDeclaredModule(t, sourceLayoutPackage, "src")
	// Same layout, installed inside a project under vendor/.
	vendoredSourceLayoutPackage := makeMagentoAuditModule(t, root, "vendor/example/module-source-layout")
	vendoredSourceLayoutModule := makeMagentoAuditDeclaredModule(t, vendoredSourceLayoutPackage, "src")
	plainComposer := t.TempDir()
	writeAuditComposer(t, plainComposer, "library", map[string]string{"example/package": "1.0.0"})
	nonRegularMarkerRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(nonRegularMarkerRoot, "bin", "magento"), 0o755); err != nil {
		t.Fatalf("create non-regular Magento marker: %v", err)
	}
	writeAuditComposer(t, nonRegularMarkerRoot, "project", map[string]string{"magento/framework": "1.0.0"})
	moduleLookingDirectory := filepath.Join(root, "app/code/Example/WithoutManifest")
	if err := os.MkdirAll(moduleLookingDirectory, 0o755); err != nil {
		t.Fatalf("create module-looking directory: %v", err)
	}

	tests := []struct {
		name           string
		startPath      string
		override       types.AuditTargetMode
		wantRecognized bool
		wantTarget     types.AuditTarget
		wantErr        bool
	}{
		{
			name:           "Magento root auto resolves full project",
			startPath:      root,
			wantRecognized: true,
			wantTarget: types.AuditTarget{
				Framework: "magento2", ProjectRoot: root, TargetPath: root, Mode: types.AuditTargetProject,
			},
		},
		{
			name:           "Mage-OS package resolves as Magento project",
			startPath:      mageOSRoot,
			wantRecognized: true,
			wantTarget: types.AuditTarget{
				Framework: "magento2", ProjectRoot: mageOSRoot, TargetPath: mageOSRoot, Mode: types.AuditTargetProject,
			},
		},
		{
			name:           "nested module auto resolves module scope",
			startPath:      filepath.Join(nestedModule, "Model"),
			wantRecognized: true,
			wantTarget: types.AuditTarget{
				Framework: "magento2", ProjectRoot: root, TargetPath: nestedModule, Mode: types.AuditTargetModule,
			},
		},
		{
			name:           "standalone module auto resolves standalone scope",
			startPath:      filepath.Join(standaloneModule, "Model"),
			wantRecognized: true,
			wantTarget: types.AuditTarget{
				Framework: "magento2", TargetPath: standaloneModule, Mode: types.AuditTargetStandalone,
			},
		},
		{
			name:           "module declared only by module.xml auto resolves module scope",
			startPath:      filepath.Join(declaredModule, "Model"),
			wantRecognized: true,
			wantTarget: types.AuditTarget{
				Framework: "magento2", ProjectRoot: root, TargetPath: declaredModule, Mode: types.AuditTargetModule,
			},
		},
		{
			name:           "forced nested module accepts module declared only by module.xml",
			startPath:      declaredModule,
			override:       types.AuditTargetModule,
			wantRecognized: true,
			wantTarget: types.AuditTarget{
				Framework: "magento2", ProjectRoot: root, TargetPath: declaredModule, Mode: types.AuditTargetModule,
			},
		},
		{
			name:           "module declared by module.xml and Composer type resolves module scope once",
			startPath:      dualSignalModule,
			wantRecognized: true,
			wantTarget: types.AuditTarget{
				Framework: "magento2", ProjectRoot: root, TargetPath: dualSignalModule, Mode: types.AuditTargetModule,
			},
		},
		{
			name:           "module declared only by module.xml outside a project auto resolves standalone scope",
			startPath:      filepath.Join(standaloneDeclaredModule, "Model"),
			wantRecognized: true,
			wantTarget: types.AuditTarget{
				Framework: "magento2", TargetPath: standaloneDeclaredModule, Mode: types.AuditTargetStandalone,
			},
		},
		{
			name:           "forced standalone accepts module declared only by module.xml outside a project",
			startPath:      standaloneDeclaredModule,
			override:       types.AuditTargetStandalone,
			wantRecognized: true,
			wantTarget: types.AuditTarget{
				Framework: "magento2", TargetPath: standaloneDeclaredModule, Mode: types.AuditTargetStandalone,
			},
		},
		{
			// A nested src/etc/module.xml must not pull the module root below the
			// Composer package boundary the caller pointed at.
			name:           "Composer module package with a src layout stays rooted at the package",
			startPath:      sourceLayoutPackage,
			wantRecognized: true,
			wantTarget: types.AuditTarget{
				Framework: "magento2", TargetPath: sourceLayoutPackage, Mode: types.AuditTargetStandalone,
			},
		},
		{
			// The Composer package boundary owns the manifest the analysis needs,
			// so it supersedes a declaration-only directory nested inside it.
			name:           "module declaration inside a Composer module package resolves the package boundary",
			startPath:      sourceLayoutModule,
			wantRecognized: true,
			wantTarget: types.AuditTarget{
				Framework: "magento2", TargetPath: sourceLayoutPackage, Mode: types.AuditTargetStandalone,
			},
		},
		{
			name:           "path below a src layout declaration resolves the Composer package boundary",
			startPath:      filepath.Join(sourceLayoutModule, "Model"),
			wantRecognized: true,
			wantTarget: types.AuditTarget{
				Framework: "magento2", TargetPath: sourceLayoutPackage, Mode: types.AuditTargetStandalone,
			},
		},
		{
			name:           "src layout package inside a project resolves the package boundary",
			startPath:      filepath.Join(vendoredSourceLayoutModule, "Model"),
			wantRecognized: true,
			wantTarget: types.AuditTarget{
				Framework: "magento2", ProjectRoot: root, TargetPath: vendoredSourceLayoutPackage, Mode: types.AuditTargetModule,
			},
		},
		{
			name:           "namespace directory holding modules remains project scope",
			startPath:      namespaceDirectory,
			wantRecognized: true,
			wantTarget: types.AuditTarget{
				Framework: "magento2", ProjectRoot: root, TargetPath: root, Mode: types.AuditTargetProject,
			},
		},
		{
			name:           "forced nested module rejects namespace directory holding modules",
			startPath:      namespaceDirectory,
			override:       types.AuditTargetModule,
			wantRecognized: true,
			wantErr:        true,
		},
		{
			name:           "module-looking directory without manifest remains project scope",
			startPath:      moduleLookingDirectory,
			wantRecognized: true,
			wantTarget: types.AuditTarget{
				Framework: "magento2", ProjectRoot: root, TargetPath: root, Mode: types.AuditTargetProject,
			},
		},
		{
			name:           "plain Composer package is not recognized",
			startPath:      plainComposer,
			wantRecognized: false,
		},
		{
			name:           "Magento package without regular CLI marker is not recognized as project",
			startPath:      nonRegularMarkerRoot,
			wantRecognized: false,
		},
		{
			name:           "forced project uses ancestor Magento root",
			startPath:      nestedModule,
			override:       types.AuditTargetProject,
			wantRecognized: true,
			wantTarget: types.AuditTarget{
				Framework: "magento2", ProjectRoot: root, TargetPath: root, Mode: types.AuditTargetProject,
			},
		},
		{
			name:           "forced nested module requires Magento project",
			startPath:      nestedModule,
			override:       types.AuditTargetModule,
			wantRecognized: true,
			wantTarget: types.AuditTarget{
				Framework: "magento2", ProjectRoot: root, TargetPath: nestedModule, Mode: types.AuditTargetModule,
			},
		},
		{
			name:           "forced standalone accepts nested module",
			startPath:      nestedModule,
			override:       types.AuditTargetStandalone,
			wantRecognized: true,
			wantTarget: types.AuditTarget{
				Framework: "magento2", TargetPath: nestedModule, Mode: types.AuditTargetStandalone,
			},
		},
		{
			name:           "forced nested module rejects standalone module",
			startPath:      standaloneModule,
			override:       types.AuditTargetModule,
			wantRecognized: true,
			wantErr:        true,
		},
		{
			name:           "forced standalone rejects project root",
			startPath:      root,
			override:       types.AuditTargetStandalone,
			wantRecognized: true,
			wantErr:        true,
		},
		{
			name:           "unknown target override is rejected",
			startPath:      root,
			override:       types.AuditTargetMode("unexpected"),
			wantRecognized: true,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, recognized, err := magento2.ResolveAuditTarget(types.AuditTargetResolveRequest{
				StartPath:    tt.startPath,
				ModeOverride: tt.override,
			})
			if recognized != tt.wantRecognized {
				t.Fatalf("recognized = %v, want %v", recognized, tt.wantRecognized)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error: %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.wantTarget {
				t.Errorf("target = %#v, want %#v", got, tt.wantTarget)
			}
		})
	}
}

func TestResolveMagentoAuditTargetCanonicalizesSymlink(t *testing.T) {
	root := makeMagentoAuditProject(t, "magento/framework")
	module := makeMagentoAuditModule(t, root, "app/code/Example/Linked")
	link := filepath.Join(t.TempDir(), "module-link")
	if err := os.Symlink(module, link); err != nil {
		t.Fatalf("create module symlink: %v", err)
	}

	got, recognized, err := magento2.ResolveAuditTarget(types.AuditTargetResolveRequest{StartPath: link})
	if err != nil {
		t.Fatalf("ResolveAuditTarget() error = %v", err)
	}
	if !recognized {
		t.Fatal("ResolveAuditTarget() did not recognize symlinked module")
	}
	want := types.AuditTarget{
		Framework: "magento2", ProjectRoot: root, TargetPath: module, Mode: types.AuditTargetModule,
	}
	if got != want {
		t.Errorf("target = %#v, want %#v", got, want)
	}
}

func makeMagentoAuditProject(t *testing.T, packageName string) string {
	t.Helper()

	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("create Magento bin directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bin, "magento"), []byte("#!/usr/bin/env php\n"), 0o755); err != nil {
		t.Fatalf("write Magento CLI marker: %v", err)
	}
	writeAuditComposer(t, root, "project", map[string]string{packageName: "1.0.0"})
	return root
}

func makeMagentoAuditModule(t *testing.T, root, relativePath string) string {
	t.Helper()

	module := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Join(module, "Model"), 0o755); err != nil {
		t.Fatalf("create nested Magento module: %v", err)
	}
	writeAuditComposer(t, module, "magento2-module", map[string]string{})
	return module
}

// makeMagentoAuditDeclaredModule builds an in-app style module that is registered
// only through etc/module.xml, without its own composer.json.
func makeMagentoAuditDeclaredModule(t *testing.T, root, relativePath string) string {
	t.Helper()

	module := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Join(module, "Model"), 0o755); err != nil {
		t.Fatalf("create declared Magento module: %v", err)
	}
	writeAuditModuleDeclaration(t, module, filepath.Base(filepath.Dir(module))+"_"+filepath.Base(module))
	return module
}

func writeAuditModuleDeclaration(t *testing.T, directory, moduleName string) {
	t.Helper()

	etc := filepath.Join(directory, "etc")
	if err := os.MkdirAll(etc, 0o755); err != nil {
		t.Fatalf("create module etc directory: %v", err)
	}
	declaration := "<?xml version=\"1.0\"?>\n<config><module name=\"" + moduleName + "\"/></config>\n"
	if err := os.WriteFile(filepath.Join(etc, "module.xml"), []byte(declaration), 0o644); err != nil {
		t.Fatalf("write module declaration: %v", err)
	}
}

func makeStandaloneMagentoAuditModule(t *testing.T) string {
	t.Helper()

	module := t.TempDir()
	if err := os.MkdirAll(filepath.Join(module, "Model"), 0o755); err != nil {
		t.Fatalf("create standalone Magento module: %v", err)
	}
	writeAuditComposer(t, module, "magento2-module", map[string]string{})
	return module
}

func writeAuditComposer(t *testing.T, directory, packageType string, require map[string]string) {
	t.Helper()

	contents, err := json.Marshal(struct {
		Type    string            `json:"type,omitempty"`
		Require map[string]string `json:"require,omitempty"`
	}{Type: packageType, Require: require})
	if err != nil {
		t.Fatalf("marshal composer manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "composer.json"), contents, 0o644); err != nil {
		t.Fatalf("write composer manifest: %v", err)
	}
}
