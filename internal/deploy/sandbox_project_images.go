package deploy

import (
	"regexp"
	"strings"
)

// SandboxProjectImageTags keeps the image references that SandboxImageTag can
// have produced for exactly this project: govard-sandbox:<slug>-<profile>-<12
// hex>. It matches the whole shape, not a prefix, so a project named "shop"
// never claims the image of a project named "shop-php".
func SandboxProjectImageTags(project string, refs []string) []string {
	pattern := regexp.MustCompile(`^govard-sandbox:` + regexp.QuoteMeta(sandboxSlug(project)) +
		`-(` + SandboxProfileBasic + `|` + SandboxProfilePHP + `|` + SandboxProfileFull + `)-[0-9a-f]{12}$`)
	var tags []string
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if pattern.MatchString(ref) {
			tags = append(tags, ref)
		}
	}
	return tags
}
