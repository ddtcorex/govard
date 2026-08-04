package engine

import (
	"context"
	"io"
	"strings"

	"github.com/pterm/pterm"
)

type UpgradeOptions struct {
	TargetVersion string
	DryRun        bool
	NoDBUpgrade   bool
	NoEnvUpdate   bool
	NoInteraction bool
	Stdout        io.Writer
	Stderr        io.Writer
	ProjectDir    string
	ProjectName   string
}

// UpgradeFunc runs one framework's upgrade pipeline (dependency bump,
// migrations, cache flush, etc.) for a target version. Implementations
// live in each framework's own package (internal/frameworks/<name>/upgrade.go).
type UpgradeFunc func(ctx context.Context, config Config, opts UpgradeOptions) error

var upgraders = map[string]UpgradeFunc{}

// RegisterUpgrader registers fn as the upgrade pipeline for framework,
// keyed the same way UpgradeFramework looks it up ("magento" alias
// included explicitly - see GetUpgrader). Called from frameworks.Register
// so a framework package can own its own upgrade pipeline instead of a
// case in this file's switch. Not safe for concurrent calls; intended
// usage is registration during package init(), before UpgradeFramework is
// ever called. Not every framework needs an entry - upgrade is
// unimplemented for most of the 14.
func RegisterUpgrader(framework string, fn UpgradeFunc) {
	upgraders[strings.ToLower(strings.TrimSpace(framework))] = fn
}

// GetUpgrader looks up the registered upgrade pipeline for framework,
// resolving the pre-existing "magento"/"wp" bare aliases the same way the
// old switch's case lists did (UpgradeFramework itself keyed on
// strings.ToLower(config.Framework) directly, not
// normalizeFrameworkManifestKey, so this mirrors that exactly rather than
// introducing new alias behavior).
func GetUpgrader(framework string) (UpgradeFunc, bool) {
	key := NormalizeFrameworkAlias(framework)
	fn, ok := upgraders[key]
	return fn, ok
}

func UpgradeFramework(ctx context.Context, config Config, opts UpgradeOptions) error {
	pterm.Info.Printf("%s Upgrade Pipeline\n", strings.ToUpper(config.Framework))

	fn, ok := GetUpgrader(config.Framework)
	if !ok {
		pterm.Warning.Printf("Upgrade for %s is not implemented yet.\n", config.Framework)
		return nil
	}
	return fn(ctx, config, opts)
}

// UpgradeFrameworkForTest exposes UpgradeFramework for tests in /tests.
func UpgradeFrameworkForTest(config Config, opts UpgradeOptions) error {
	return UpgradeFramework(context.Background(), config, opts)
}
