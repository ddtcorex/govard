package bootstrap

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"govard/internal/conventions"

	"github.com/pterm/pterm"
)

type PrestaShopBootstrap struct {
	Options Options
}

func NewPrestaShopBootstrap(opts Options) *PrestaShopBootstrap {
	return &PrestaShopBootstrap{Options: opts}
}

func (p *PrestaShopBootstrap) Name() string {
	return "prestashop"
}

func (p *PrestaShopBootstrap) SupportsFreshInstall() bool {
	return false
}

func (p *PrestaShopBootstrap) SupportsClone() bool {
	return true
}

func (p *PrestaShopBootstrap) FreshCommands() []string {
	return []string{}
}

func (p *PrestaShopBootstrap) CreateProject(projectDir string) error {
	return fmt.Errorf("fresh install not supported for PrestaShop, use --clone instead")
}

func (p *PrestaShopBootstrap) Install(projectDir string) error {
	pterm.Info.Println("Setting up PrestaShop...")
	pterm.Success.Println("PrestaShop setup completed")
	return nil
}

func (p *PrestaShopBootstrap) Configure(projectDir string) error {
	pterm.Info.Println("Configuring PrestaShop environment...")

	parametersPath := filepath.Join(projectDir, filepath.FromSlash(conventions.PrestaShopParametersFile))
	if _, err := os.Stat(parametersPath); err == nil {
		if err := p.patchParametersFile(parametersPath); err != nil {
			pterm.Warning.Printf("Failed to patch parameters.php: %v\n", err)
		}
	}

	pterm.Success.Println("PrestaShop configured successfully")
	return nil
}

func (p *PrestaShopBootstrap) PostClone(projectDir string) error {
	pterm.Info.Println("Setting up cloned PrestaShop project...")

	p.ensureWritableDirs(projectDir)

	parametersPath := filepath.Join(projectDir, filepath.FromSlash(conventions.PrestaShopParametersFile))
	if _, err := os.Stat(parametersPath); err == nil {
		if err := p.patchParametersFile(parametersPath); err != nil {
			pterm.Warning.Printf("Failed to patch parameters.php: %v\n", err)
		}
	} else if os.IsNotExist(err) {
		if err := p.createParametersFile(parametersPath); err != nil {
			pterm.Warning.Printf("Failed to create parameters.php: %v\n", err)
		} else {
			pterm.Success.Println("Created app/config/parameters.php")
		}
	}

	pterm.Success.Println("Post-clone setup completed")
	return nil
}

func (p *PrestaShopBootstrap) resolveDBConfig() (host, user, pass, name, prefix string) {
	host = strings.TrimSpace(p.Options.DBHost)
	if host == "" {
		host = conventions.DefaultDBHost
	}
	user = strings.TrimSpace(p.Options.DBUser)
	if user == "" {
		user = conventions.DefaultPrestaShopDBUser
	}
	pass = p.Options.DBPass
	if pass == "" {
		pass = conventions.DefaultPrestaShopDBPass
	}
	name = strings.TrimSpace(p.Options.DBName)
	if name == "" {
		name = conventions.DefaultPrestaShopDBName
	}
	prefix = strings.TrimSpace(p.Options.TablePrefix)
	if prefix == "" {
		prefix = conventions.DefaultPrestaShopTablePrefix
	}
	return host, user, pass, name, prefix
}

func (p *PrestaShopBootstrap) patchParametersFile(parametersPath string) error {
	data, err := os.ReadFile(parametersPath)
	if err != nil {
		return fmt.Errorf("read parameters.php: %w", err)
	}

	host, user, pass, name, prefix := p.resolveDBConfig()
	content := string(data)
	content = patchPrestaShopParameter(content, "database_host", host)
	content = patchPrestaShopParameter(content, "database_user", user)
	content = patchPrestaShopParameter(content, "database_password", pass)
	content = patchPrestaShopParameter(content, "database_name", name)
	content = patchPrestaShopParameter(content, "database_prefix", prefix)

	if content == string(data) {
		return nil
	}

	return os.WriteFile(parametersPath, []byte(content), conventions.DefaultFilePerm)
}

// createParametersFile fabricates a best-effort app/config/parameters.php when a
// cloned PrestaShop project has none at all. This key set is stable across
// PrestaShop 1.7 GA through 8.x/9.x, but it is explicitly a fallback, not a
// substitute for PrestaShop's real installer.
func (p *PrestaShopBootstrap) createParametersFile(parametersPath string) error {
	host, user, pass, name, prefix := p.resolveDBConfig()

	secret, err := generatePrestaShopSecret()
	if err != nil {
		return fmt.Errorf("generate secret: %w", err)
	}
	cookieKey, err := generatePrestaShopSecret()
	if err != nil {
		return fmt.Errorf("generate cookie_key: %w", err)
	}
	cookieIV, err := generatePrestaShopSecret()
	if err != nil {
		return fmt.Errorf("generate cookie_iv: %w", err)
	}
	newCookieKey, err := generatePrestaShopSecret()
	if err != nil {
		return fmt.Errorf("generate new_cookie_key: %w", err)
	}

	content := fmt.Sprintf(`<?php

return array (
  'parameters' =>
  array (
    'database_host' => %s,
    'database_port' => '',
    'database_name' => %s,
    'database_user' => %s,
    'database_password' => %s,
    'database_prefix' => %s,
    'database_engine' => 'InnoDB',
    'mailer_transport' => 'smtp',
    'mailer_host' => 'mailpit',
    'mailer_user' => NULL,
    'mailer_password' => NULL,
    'secret' => %s,
    'cookie_key' => %s,
    'cookie_iv' => %s,
    'new_cookie_key' => %s,
    'ps_caching' => 'CacheMemcache',
    'ps_cache_enable' => false,
    'ps_creation_date' => %s,
  ),
);
`,
		phpQuote(host), phpQuote(name), phpQuote(user), phpQuote(pass), phpQuote(prefix),
		phpQuote(secret), phpQuote(cookieKey), phpQuote(cookieIV), phpQuote(newCookieKey),
		phpQuote(time.Now().Format("2006-01-02")))

	if err := os.MkdirAll(filepath.Dir(parametersPath), conventions.DefaultDirPerm); err != nil {
		return fmt.Errorf("create app/config directory: %w", err)
	}

	return os.WriteFile(parametersPath, []byte(content), conventions.DefaultFilePerm)
}

func (p *PrestaShopBootstrap) ensureWritableDirs(projectDir string) {
	for _, dir := range []string{filepath.Join("var", "cache"), filepath.Join("var", "logs")} {
		_ = os.MkdirAll(filepath.Join(projectDir, dir), conventions.DefaultDirPerm)
	}

	for _, dir := range []string{filepath.Join("var", "cache"), filepath.Join("var", "logs"), "img", "upload", "download", "config"} {
		dirPath := filepath.Join(projectDir, dir)
		if _, err := os.Stat(dirPath); err != nil {
			continue
		}
		_ = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				_ = os.Chmod(path, conventions.PublicDirPerm)
			} else {
				_ = os.Chmod(path, conventions.PublicFilePerm)
			}
			return nil
		})
	}
}

func phpQuote(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	return "'" + escaped + "'"
}

// patchPrestaShopParameter overwrites the value of an existing 'key' => '...'
// entry in a PrestaShop parameters.php array literal, leaving every other key
// untouched. It does not add the key if it isn't already present, since we don't
// want to guess at a schema we can't fully verify.
func patchPrestaShopParameter(content, key, value string) string {
	pattern := regexp.MustCompile(`'` + regexp.QuoteMeta(key) + `'\s*=>\s*'(?:[^'\\]|\\.)*'`)
	replacement := "'" + key + "' => " + phpQuote(value)
	return pattern.ReplaceAllStringFunc(content, func(string) string {
		return replacement
	})
}

func generatePrestaShopSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
