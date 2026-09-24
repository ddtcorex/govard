package tests

import (
	"strings"
	"testing"

	"govard/internal/deploy"
	"govard/internal/frameworks/laravel"
	"govard/internal/frameworks/magento2"
	"govard/internal/frameworks/symfony"
	"govard/internal/frameworks/wordpress"
)

// The cache flush belongs to the application the web server is serving, not to
// the release being built. With a symlink layout the release becomes `current`
// before the step runs, so the two directories coincide and the defect is
// invisible; with an in-place layout the docroot is a *different* directory the
// release copies into, and a flush in the release deletes a cache nothing reads
// while the served application keeps the old one.
//
// Measured on a real in-place target (release 16):
// `.deployer/releases/16/var/cache` (27 MB) was flushed while
// `public_html/var/cache` (34 MB) and `public_html/var/page_cache` (9.1 MB)
// survived, and `app/etc/env.php` declared no cache backend other than the file
// default — so nothing at all was invalidated for the served application.
func TestFrameworkCacheFlushRunsAgainstTheServedApplication(t *testing.T) {
	for name, recipe := range map[string]deploy.Recipe{
		"magento2":  magento2.DeployRecipe(),
		"laravel":   laravel.DeployRecipe(),
		"symfony":   symfony.DeployRecipe(),
		"wordpress": wordpress.DeployRecipe(),
	} {
		t.Run(name, func(t *testing.T) {
			command := recipe.Task(deploy.TaskAppCacheFlush).Command
			if !strings.Contains(command, "{{current_path}}") {
				t.Errorf("app:cache:flush must run where the application is served:\n%s", command)
			}
			if strings.Contains(command, "{{release_path}}") {
				t.Errorf("app:cache:flush must not run in the release being built:\n%s", command)
			}
		})
	}
}
