package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"govard/internal/runtime"
)

var domainCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapDocker),
	},
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
	Annotations: map[string]string{
		// Listing reads .govard.yml and prints it: `domain add`/`remove` mutate
		// the config and `govard env up` applies it to the stack, but neither
		// happens here, so this one runs on a host with no container runtime.
		runtime.AnnotationRequires: string(runtime.CapNone),
	},
	Use:   "list",
	Short: "List all domains for the project",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		config := loadConfig()
		pterm.DefaultSection.Println("Project Domains")
		pterm.Info.Printfln("Primary: %s", pterm.Cyan(config.Domain))
		fmt.Println()

		if len(config.ExtraDomains) > 0 {
			pterm.DefaultSection.WithLevel(2).Println("Extra Domains")
			var items []pterm.BulletListItem
			for _, d := range config.ExtraDomains {
				items = append(items, pterm.BulletListItem{Level: 0, Text: d})
			}
			_ = pterm.DefaultBulletList.WithItems(items).Render()
		}

		if len(config.StoreDomains) > 0 {
			pterm.DefaultSection.WithLevel(2).Println("Store Domains")
			hosts := make([]string, 0, len(config.StoreDomains))
			for host := range config.StoreDomains {
				hosts = append(hosts, host)
			}
			sort.Strings(hosts)

			tableData := pterm.TableData{
				{"DOMAIN", "CODE", "TYPE"},
			}
			for _, host := range hosts {
				m := config.StoreDomains[host]
				tableData = append(tableData, []string{host, m.Code, m.Type})
			}
			_ = pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
		}

		return nil
	},
}

func init() {
	domainCmd.AddCommand(domainAddCmd)
	domainCmd.AddCommand(domainRemoveCmd)
	domainCmd.AddCommand(domainListCmd)
}
