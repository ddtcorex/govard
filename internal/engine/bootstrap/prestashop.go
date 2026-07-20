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

	if err := p.updateShopURL(); err != nil {
		pterm.Warning.Printf("Failed to update shop URL: %v\n", err)
	}
	if err := p.enableSSL(); err != nil {
		pterm.Warning.Printf("Failed to enable SSL: %v\n", err)
	}
	if err := p.configureMail(); err != nil {
		pterm.Warning.Printf("Failed to configure mail relay: %v\n", err)
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

	if err := p.updateShopURL(); err != nil {
		pterm.Warning.Printf("Failed to update shop URL: %v\n", err)
	}
	if err := p.enableSSL(); err != nil {
		pterm.Warning.Printf("Failed to enable SSL: %v\n", err)
	}
	if err := p.configureMail(); err != nil {
		pterm.Warning.Printf("Failed to configure mail relay: %v\n", err)
	}

	pterm.Success.Println("Post-clone setup completed")
	return nil
}

// updateShopURL points the shop's primary domain at the local govard domain. Without
// this, PrestaShop keeps serving/redirecting to whatever domain was set on the source
// environment the project was cloned from, since (unlike WordPress's siteurl/home
// options or Shopware's sales channel domain) nothing else in the bootstrap flow touches
// this. PrestaShop keeps the domain in two separate places that don't auto-sync with
// each other: the ps_shop_url table (id_shop_url = 1) and a denormalized copy in
// ps_configuration (PS_SHOP_DOMAIN/PS_SHOP_DOMAIN_SSL, read by Tools::getShopDomain*())
// — both need updating or pages/redirects keep resolving against the old domain.
func (p *PrestaShopBootstrap) updateShopURL() error {
	domain := strings.TrimSpace(p.Options.Domain)
	projectName := strings.TrimSpace(p.Options.ProjectName)
	if domain == "" || projectName == "" {
		return nil
	}

	_, user, pass, name, prefix := p.resolveDBConfig()
	containerName := projectName + conventions.DBSuffix

	return RunSQLViaDockerExec(containerName, user, pass, name, buildPrestaShopShopURLSQL(prefix, domain))
}

func BuildPrestaShopShopURLSQLForTest(prefix, domain string) string {
	return buildPrestaShopShopURLSQL(prefix, domain)
}

// buildPrestaShopShopURLSQL returns the SQL statements that set the shop's primary
// domain in both ps_shop_url (id_shop_url = 1) and its ps_configuration denormalized
// copy (PS_SHOP_DOMAIN/PS_SHOP_DOMAIN_SSL) to the given domain.
func buildPrestaShopShopURLSQL(prefix, domain string) string {
	quoted := singleQuoteEscape(domain)
	return fmt.Sprintf(
		"UPDATE %[1]sshop_url SET domain = %[2]s, domain_ssl = %[2]s WHERE id_shop_url = 1; "+
			"UPDATE %[1]sconfiguration SET value = %[2]s WHERE name IN ('PS_SHOP_DOMAIN', 'PS_SHOP_DOMAIN_SSL');",
		prefix, quoted,
	)
}

// enableSSL forces PS_SSL_ENABLED/PS_SSL_ENABLED_EVERYWHERE on. Govard always serves
// projects over HTTPS via its Caddy proxy, so this is a safe, universal default for any
// govard-managed PrestaShop project rather than something project-specific.
func (p *PrestaShopBootstrap) enableSSL() error {
	projectName := strings.TrimSpace(p.Options.ProjectName)
	if projectName == "" {
		return nil
	}

	_, user, pass, name, prefix := p.resolveDBConfig()
	containerName := projectName + conventions.DBSuffix

	return RunSQLViaDockerExec(containerName, user, pass, name, buildPrestaShopEnableSSLSQL(prefix))
}

func BuildPrestaShopEnableSSLSQLForTest(prefix string) string {
	return buildPrestaShopEnableSSLSQL(prefix)
}

// buildPrestaShopEnableSSLSQL returns the SQL statement that forces SSL on for the shop.
func buildPrestaShopEnableSSLSQL(prefix string) string {
	return fmt.Sprintf(
		"UPDATE %sconfiguration SET value = 1 WHERE name IN ('PS_SSL_ENABLED', 'PS_SSL_ENABLED_EVERYWHERE');",
		prefix,
	)
}

// configureMail points the shop's customer-facing mail relay (PS_MAIL_SERVER/
// PS_MAIL_USER/PS_MAIL_SMTP_PORT — distinct from parameters.php's Symfony-mailer keys
// already handled by patchParametersFile/createParametersFile) at govard's shared local
// mail catcher. Universal for any govard-managed PrestaShop project, same reasoning as
// enableSSL: govard always provides this catcher, regardless of project-specific
// payment/module configuration.
func (p *PrestaShopBootstrap) configureMail() error {
	projectName := strings.TrimSpace(p.Options.ProjectName)
	if projectName == "" {
		return nil
	}

	_, user, pass, name, prefix := p.resolveDBConfig()
	containerName := projectName + conventions.DBSuffix

	return RunSQLViaDockerExec(containerName, user, pass, name, buildPrestaShopMailSQL(prefix))
}

func BuildPrestaShopMailSQLForTest(prefix string) string {
	return buildPrestaShopMailSQL(prefix)
}

// buildPrestaShopMailSQL returns the SQL statements that point PS_MAIL_SERVER/
// PS_MAIL_USER/PS_MAIL_SMTP_PORT at govard's shared local mail catcher ('mail', no auth,
// port 1025 — matching the mailer_host/mailer_port fabricated into parameters.php).
func buildPrestaShopMailSQL(prefix string) string {
	return fmt.Sprintf(
		"UPDATE %[1]sconfiguration SET value = 'mail' WHERE name LIKE 'PS_MAIL_SERVER'; "+
			"UPDATE %[1]sconfiguration SET value = '' WHERE name LIKE 'PS_MAIL_USER'; "+
			"UPDATE %[1]sconfiguration SET value = 1025 WHERE name LIKE 'PS_MAIL_SMTP_PORT';",
		prefix,
	)
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

// resolveSecrets returns the secret/cookie_key/cookie_iv/new_cookie_key values to write
// into a fabricated parameters.php. Each one is taken from the remote-probed value on
// Options when present (so any module data encrypted under the remote's keys stays
// decryptable locally), falling back to a freshly generated random value otherwise.
func (p *PrestaShopBootstrap) resolveSecrets() (secret, cookieKey, cookieIV, newCookieKey string, err error) {
	secret, err = resolveOrGeneratePrestaShopSecret(p.Options.PrestaShopSecret)
	if err != nil {
		return "", "", "", "", fmt.Errorf("generate secret: %w", err)
	}
	cookieKey, err = resolveOrGeneratePrestaShopSecret(p.Options.PrestaShopCookieKey)
	if err != nil {
		return "", "", "", "", fmt.Errorf("generate cookie_key: %w", err)
	}
	cookieIV, err = resolveOrGeneratePrestaShopSecret(p.Options.PrestaShopCookieIV)
	if err != nil {
		return "", "", "", "", fmt.Errorf("generate cookie_iv: %w", err)
	}
	newCookieKey, err = resolveOrGeneratePrestaShopSecret(p.Options.PrestaShopNewCookieKey)
	if err != nil {
		return "", "", "", "", fmt.Errorf("generate new_cookie_key: %w", err)
	}
	return secret, cookieKey, cookieIV, newCookieKey, nil
}

func resolveOrGeneratePrestaShopSecret(remoteValue string) (string, error) {
	if trimmed := strings.TrimSpace(remoteValue); trimmed != "" {
		return trimmed, nil
	}
	return generatePrestaShopSecret()
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

	secret, cookieKey, cookieIV, newCookieKey, err := p.resolveSecrets()
	if err != nil {
		return err
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
    'mailer_host' => 'mail',
    'mailer_port' => 1025,
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
		singleQuoteEscape(host), singleQuoteEscape(name), singleQuoteEscape(user), singleQuoteEscape(pass), singleQuoteEscape(prefix),
		singleQuoteEscape(secret), singleQuoteEscape(cookieKey), singleQuoteEscape(cookieIV), singleQuoteEscape(newCookieKey),
		singleQuoteEscape(time.Now().Format("2006-01-02")))

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

// singleQuoteEscape escapes backslashes and single quotes for embedding a value inside
// a single-quoted literal — the escaping rules are identical for a PHP array literal
// and a MySQL string literal, so this is shared by both parameters.php generation/patching
// and the shop URL SQL statement.
func singleQuoteEscape(value string) string {
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
	replacement := "'" + key + "' => " + singleQuoteEscape(value)
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
