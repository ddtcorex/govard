package cmd

import (
	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var trustCmd = &cobra.Command{
	Use:   "trust",
	Short: "Trust the local CA for SSL certificates",
	RunE: func(cmd *cobra.Command, args []string) error {
		pterm.DefaultHeader.Println("Govard SSL Trust Store")

		if err := engine.TrustCA(); err != nil {
			return err
		}

		pterm.Success.Println("🛡️ Root CA successfully installed into system trust store!")
		return nil
	},
}
