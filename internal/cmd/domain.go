package cmd

import (
	"fmt"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var domainCmd = &cobra.Command{
	Use:   "domain",
	Short: "Manage additional domains for the project",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var domainAddCmd = &cobra.Command{
	Use:   "add [domain]",
	Short: "Add an extra domain to the project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadWritableConfig()
		if err != nil {
			return err
		}
		newDomain := strings.TrimSpace(args[0])
		if newDomain == "" {
			return fmt.Errorf("domain cannot be empty")
		}

		if newDomain == config.Domain {
			pterm.Info.Printf("Domain %s is already the primary domain.\n", newDomain)
			return nil
		}

		for _, d := range config.ExtraDomains {
			if d == newDomain {
				pterm.Info.Printf("Domain %s is already in extra domains.\n", newDomain)
				return nil
			}
		}

		config.ExtraDomains = append(config.ExtraDomains, newDomain)
		saveConfig(config)
		pterm.Success.Printf("Domain %s added to .govard.yml. Run 'govard env up' to apply changes.\n", newDomain)
		return nil
	},
}

var domainRemoveCmd = &cobra.Command{
	Use:   "remove [domain]",
	Short: "Remove an extra domain from the project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadWritableConfig()
		if err != nil {
			return err
		}
		toRemove := strings.TrimSpace(args[0])

		var updated []string
		found := false
		for _, d := range config.ExtraDomains {
			if d == toRemove {
				found = true
				continue
			}
			updated = append(updated, d)
		}

		if !found {
			return fmt.Errorf("domain %s not found in extra domains", toRemove)
		}

		config.ExtraDomains = updated
		saveConfig(config)
		pterm.Success.Printf("Domain %s removed from .govard.yml. Run 'govard env up' to apply changes.\n", toRemove)
		return nil
	},
}

var domainListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all domains for the project",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		config := loadConfig()
		pterm.DefaultSection.Println("Project Domains")
		pterm.Info.Printf("Primary: %s\n", config.Domain)
		if len(config.ExtraDomains) > 0 {
			pterm.Info.Println("Extra Domains:")
			for _, d := range config.ExtraDomains {
				_ = pterm.DefaultBulletList.WithItems([]pterm.BulletListItem{
					{Level: 0, Text: d},
				}).Render()
			}
		}
		return nil
	},
}

func init() {
	domainCmd.AddCommand(domainAddCmd)
	domainCmd.AddCommand(domainRemoveCmd)
	domainCmd.AddCommand(domainListCmd)
}
