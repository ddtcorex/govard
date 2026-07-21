package engine

import (
	"fmt"
	"govard/internal/conventions"
	"sort"
	"strings"
)

type Features struct {
	Cache      bool `yaml:"-"`
	Search     bool `yaml:"-"`
	Queue      bool `yaml:"-"`
	Varnish    bool `yaml:"varnish,omitempty"`
	Xdebug     bool `yaml:"xdebug,omitempty"`
	Isolated   bool `yaml:"isolated,omitempty"`
	LiveReload bool `yaml:"livereload,omitempty"`
	MFTF       bool `yaml:"mftf,omitempty"`
}

type Services struct {
	WebServer string `yaml:"web_server,omitempty"`
	DB        string `yaml:"db,omitempty"`
	Cache     string `yaml:"cache,omitempty"`
	Search    string `yaml:"search,omitempty"`
	Queue     string `yaml:"queue,omitempty"`
}

type Stack struct {
	WebRoot         string   `yaml:"web_root,omitempty"`
	NginxVersion    string   `yaml:"nginx_version,omitempty"`
	ApacheVersion   string   `yaml:"apache_version,omitempty"`
	PHPVersion      string   `yaml:"php_version,omitempty"`
	NodeVersion     string   `yaml:"node_version,omitempty"`
	ComposerVersion string   `yaml:"composer_version,omitempty"`
	VarnishVersion  string   `yaml:"varnish_version,omitempty"`
	DBVersion       string   `yaml:"db_version,omitempty"`
	CacheVersion    string   `yaml:"cache_version,omitempty"`
	SearchVersion   string   `yaml:"search_version,omitempty"`
	QueueVersion    string   `yaml:"queue_version,omitempty"`
	XdebugSession   string   `yaml:"xdebug_session,omitempty"`
	XdebugVersion   string   `yaml:"xdebug_version,omitempty"`
	WebServer       string   `yaml:"web_server,omitempty"`
	UserID          int      `yaml:"user_id,omitempty"`
	GroupID         int      `yaml:"group_id,omitempty"`
	Services        Services `yaml:"services,omitempty"`
	Features        Features `yaml:"features,omitempty"`
	ChownDirList    []string `yaml:"chown_dir_list,omitempty"`
}

type Config struct {
	ProjectName      string              `yaml:"project_name"`
	Profile          string              `yaml:"profile,omitempty"`
	Framework        string              `yaml:"framework"`
	FrameworkVersion string              `yaml:"framework_version,omitempty"`
	Domain           string              `yaml:"domain"`
	ExtraDomains     []string            `yaml:"extra_domains,omitempty"`
	StoreDomains     StoreDomainMappings `yaml:"store_domains,omitempty"`
	TablePrefix      string              `yaml:"table_prefix,omitempty"`
	LinkedProjects   []string            `yaml:"linked_projects,omitempty"`

	Lock              LockConfig              `yaml:"lock,omitempty"`
	BlueprintRegistry BlueprintRegistryConfig `yaml:"blueprint_registry,omitempty"`
	Stack             Stack                   `yaml:"stack"`
	Remotes           RemoteConfigMap         `yaml:"remotes,omitempty"`
	Hooks             map[string][]HookStep   `yaml:"hooks,omitempty"`
}

type LockConfig struct {
	Strict       bool     `yaml:"strict,omitempty"`
	IgnoreFields []string `yaml:"ignore_fields,omitempty"`
}

type BlueprintRegistryConfig struct {
	Provider string `yaml:"provider,omitempty"`
	URL      string `yaml:"url,omitempty"`
	Ref      string `yaml:"ref,omitempty"`
	Checksum string `yaml:"checksum,omitempty"`
	Trusted  bool   `yaml:"trusted,omitempty"`
}

type HookStep struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
}

// AllDomains returns the primary Domain followed by any non-duplicate
// ExtraDomains and StoreDomains. It trims whitespace from each domain,
// skips empty strings, and keeps StoreDomains in sorted order so
// downstream config rendering stays deterministic.
func (c Config) AllDomains() []string {
	seen := make(map[string]bool)
	domains := []string{}

	primary := strings.TrimSpace(c.Domain)
	if primary != "" {
		domains = append(domains, primary)
		seen[primary] = true
	}

	for _, domain := range c.ExtraDomains {
		trimmed := strings.TrimSpace(domain)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		domains = append(domains, trimmed)
		seen[trimmed] = true
	}

	storeDomains := make([]string, 0, len(c.StoreDomains))
	for domain := range c.StoreDomains {
		storeDomains = append(storeDomains, domain)
	}
	sort.Strings(storeDomains)

	for _, domain := range storeDomains {
		trimmed := strings.TrimSpace(domain)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		domains = append(domains, trimmed)
		seen[trimmed] = true
	}

	return domains
}

func (c Config) ResolveProjectExecUser(fallback string) string {
	if c.Stack.UserID > 0 && c.Stack.GroupID > 0 {
		return fmt.Sprintf("%d:%d", c.Stack.UserID, c.Stack.GroupID)
	}
	return fallback
}

// GetDefaultChownDirList returns the standard list of directories that require
// consistent ownership for developer workflows. For instance, Magento 2 projects
// require /var/www/html to be writable for generated code and static assets.
func GetDefaultChownDirList(framework string) []string {
	// Note: /home/www-data/.ssh is intentionally NOT included here.
	// The SSH directory is always mounted :ro, so chown would fail.
	list := []string{"/bash_history"}
	if framework == "magento2" || framework == "mageos" {
		list = append(list, conventions.DefaultWorkDir, conventions.HomeWWWData+"/.cache/composer")
	}
	return list
}
