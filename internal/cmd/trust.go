package cmd

import (
	"fmt"
	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"govard/internal/runtime"
)

var trustCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapNone),
	},
	Use:   "trust",
	Short: "Trust the local CA for SSL certificates",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println()
		pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Println(" Govard SSL Trust Store ")
		fmt.Println()

		if err := engine.TrustCA(); err != nil {
			return err
		}

		pterm.Success.Println("🛡️ Root CA successfully installed into system trust store!")
		return nil
	},
}
