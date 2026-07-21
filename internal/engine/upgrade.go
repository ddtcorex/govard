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

func UpgradeFramework(ctx context.Context, config Config, opts UpgradeOptions) error {
	pterm.Info.Printf("%s Upgrade Pipeline\n", strings.ToUpper(config.Framework))

	switch strings.ToLower(config.Framework) {
	case "magento2", "magento":
		return upgradeMagento2(ctx, config, opts, magento2UpgradeVariant)
	case "mageos":
		return upgradeMagento2(ctx, config, opts, mageOSUpgradeVariant)
	case "magento1":
		return upgradeMagento1(ctx, config, opts)
	case "laravel":
		return upgradeLaravel(ctx, config, opts)
	case "symfony":
		return upgradeSymfony(ctx, config, opts)
	case "wordpress", "wp":
		return upgradeWordPress(ctx, config, opts)
	default:
		pterm.Warning.Printf("Upgrade for %s is not implemented yet.\n", config.Framework)
		return nil
	}
}

// UpgradeFrameworkForTest exposes UpgradeFramework for tests in /tests.
func UpgradeFrameworkForTest(config Config, opts UpgradeOptions) error {
	return UpgradeFramework(context.Background(), config, opts)
}
