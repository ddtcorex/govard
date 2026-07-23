package bootstrap

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"govard/internal/conventions"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pterm/pterm"
)

const (
	emdashTemplateArchiveURL = "https://codeload.github.com/emdash-cms/templates/tar.gz/refs/heads/main"
	emdashTemplatePrefix     = "templates-main/starter/"
)

type EmdashBootstrap struct {
	Options Options
}

func NewEmdashBootstrap(opts Options) *EmdashBootstrap {
	return &EmdashBootstrap{Options: opts}
}

func (e *EmdashBootstrap) Name() string {
	return "emdash"
}

func (e *EmdashBootstrap) SupportsFreshInstall() bool {
	return true
}

func (e *EmdashBootstrap) SupportsClone() bool {
	return false
}

func (e *EmdashBootstrap) FreshCommands() []string {
	return []string{
		"download starter template from emdash-cms/templates",
	}
}

func (e *EmdashBootstrap) CreateProject(projectDir string) error {
	pterm.Info.Println("Creating fresh Emdash project...")

	if err := removeProjectContents(projectDir); err != nil {
		return err
	}
	if err := downloadAndExtractStarterTemplate(projectDir); err != nil {
		return err
	}
	if err := rewriteEmdashPackageName(projectDir); err != nil {
		return err
	}
	if err := patchEmdashAstroConfig(projectDir); err != nil {
		return err
	}
	if err := writeEmdashPasskeyShim(projectDir); err != nil {
		return err
	}

	pterm.Success.Println("Emdash project created successfully")
	return nil
}

func (e *EmdashBootstrap) Install(projectDir string) error {
	pterm.Info.Println("Emdash dependencies will install automatically when the Govard environment starts.")
	return nil
}

func (e *EmdashBootstrap) Configure(projectDir string) error {
	pterm.Success.Println("Emdash configured successfully")
	return nil
}

func (e *EmdashBootstrap) PostClone(projectDir string) error {
	return fmt.Errorf("post-clone bootstrap is not supported for emdash yet")
}

func downloadAndExtractStarterTemplate(projectDir string) error {
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(emdashTemplateArchiveURL)
	if err != nil {
		return fmt.Errorf("download emdash starter template: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download emdash starter template: unexpected status %s", resp.Status)
	}

	gzipReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("read emdash starter template archive: %w", err)
	}
	defer func() {
		if closeErr := gzipReader.Close(); closeErr != nil {
			pterm.Warning.Printf("Could not close emdash starter template archive reader: %v\n", closeErr)
		}
	}()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("extract emdash starter template: %w", err)
		}

		if !strings.HasPrefix(header.Name, emdashTemplatePrefix) {
			continue
		}

		relativePath := strings.TrimPrefix(header.Name, emdashTemplatePrefix)
		relativePath = strings.TrimPrefix(relativePath, "/")
		if relativePath == "" {
			continue
		}

		targetPath := filepath.Join(projectDir, filepath.FromSlash(relativePath))
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, conventions.DefaultDirPerm); err != nil {
				return fmt.Errorf("create template directory %s: %w", targetPath, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), conventions.DefaultDirPerm); err != nil {
				return fmt.Errorf("create template parent directory %s: %w", targetPath, err)
			}
			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("create template file %s: %w", targetPath, err)
			}
			if _, err := io.Copy(file, tarReader); err != nil {
				closeErr := file.Close()
				return fmt.Errorf("write template file %s: %w", targetPath, errors.Join(err, closeErr))
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("close template file %s: %w", targetPath, err)
			}
		}
	}

	return nil
}

func rewriteEmdashPackageName(projectDir string) error {
	packagePath := filepath.Join(projectDir, "package.json")
	data, err := os.ReadFile(packagePath)
	if err != nil {
		return fmt.Errorf("read emdash package.json: %w", err)
	}

	var pkg map[string]interface{}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return fmt.Errorf("parse emdash package.json: %w", err)
	}

	projectName := filepath.Base(projectDir)
	pkg["name"] = projectName

	if _, err := os.Stat(filepath.Join(projectDir, "seed", "seed.json")); err == nil {
		pkg["emdash"] = map[string]string{
			"label": "Starter",
			"seed":  "seed/seed.json",
		}
	}

	updated, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal emdash package.json: %w", err)
	}
	updated = append(updated, '\n')
	if err := os.WriteFile(packagePath, updated, conventions.DefaultFilePerm); err != nil {
		return fmt.Errorf("write emdash package.json: %w", err)
	}

	return nil
}

func patchEmdashAstroConfig(projectDir string) error {
	configPath := filepath.Join(projectDir, "astro.config.mjs")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read emdash astro config: %w", err)
	}

	content := string(data)
	const fileURLToPathImport = "import { fileURLToPath } from \"node:url\";\n"
	const defineConfigImportAnchor = "import { defineConfig } from \"astro/config\";\n"
	if !strings.Contains(content, fileURLToPathImport) {
		if strings.Contains(content, defineConfigImportAnchor) {
			content = strings.Replace(content, defineConfigImportAnchor, defineConfigImportAnchor+fileURLToPathImport, 1)
		} else {
			return fmt.Errorf("patch emdash astro config: expected defineConfig import anchor not found")
		}
	}

	const trustedDomainLine = "const trustedForwardedDomain = process.env.GOVARD_TRUSTED_DOMAIN?.trim();\n"
	content = strings.ReplaceAll(content, trustedDomainLine, "")

	const importAnchor = "import { sqlite } from \"emdash/db\";\n"
	if strings.Contains(content, importAnchor) {
		content = strings.Replace(content, importAnchor, importAnchor+"\n"+trustedDomainLine, 1)
	} else {
		return fmt.Errorf("patch emdash astro config: expected import anchor not found")
	}

	const securitySnippet = "\tsecurity: trustedForwardedDomain\n\t\t? {\n\t\t\t\tallowedDomains: [{ hostname: trustedForwardedDomain, protocol: \"https\" }],\n\t\t\t}\n\t\t: undefined,\n"
	content = strings.ReplaceAll(content, securitySnippet, "")

	const viteAliasSnippet = "\tvite: {\n\t\tresolve: {\n\t\t\talias: {\n\t\t\t\t\"#auth/passkey-config.js\": fileURLToPath(new URL(\"./src/govard/passkey-config.ts\", import.meta.url)),\n\t\t\t},\n\t\t},\n\t},\n"
	content = strings.ReplaceAll(content, viteAliasSnippet, "")

	const configAnchor = "\tdevToolbar: { enabled: false },\n"
	if strings.Contains(content, configAnchor) {
		content = strings.Replace(content, configAnchor, securitySnippet+viteAliasSnippet+configAnchor, 1)
	} else {
		return fmt.Errorf("patch emdash astro config: expected config anchor not found")
	}

	if err := os.WriteFile(configPath, []byte(content), conventions.DefaultFilePerm); err != nil {
		return fmt.Errorf("write emdash astro config: %w", err)
	}

	return nil
}

func PatchEmdashAstroConfigForTest(projectDir string) error {
	return patchEmdashAstroConfig(projectDir)
}

func writeEmdashPasskeyShim(projectDir string) error {
	shimPath := filepath.Join(projectDir, "src", "govard", "passkey-config.ts")
	if err := os.MkdirAll(filepath.Dir(shimPath), conventions.DefaultDirPerm); err != nil {
		return fmt.Errorf("create emdash passkey shim dir: %w", err)
	}

	content := `export interface PasskeyConfig {
	rpName: string;
	rpId: string;
	origin: string;
}

export function getPasskeyConfig(url: URL, siteName?: string): PasskeyConfig {
	const trustedForwardedDomain = process.env.GOVARD_TRUSTED_DOMAIN?.trim();
	const hostname = trustedForwardedDomain || url.hostname;
	const origin = trustedForwardedDomain ? "https://" + trustedForwardedDomain : url.origin;

	return {
		rpName: siteName || hostname,
		rpId: hostname,
		origin,
	};
}
`
	if err := os.WriteFile(shimPath, []byte(content), conventions.DefaultFilePerm); err != nil {
		return fmt.Errorf("write emdash passkey shim: %w", err)
	}

	return nil
}

func WriteEmdashPasskeyShimForTest(projectDir string) error {
	return writeEmdashPasskeyShim(projectDir)
}
