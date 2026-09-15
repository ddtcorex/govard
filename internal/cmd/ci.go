package cmd

import (
	"github.com/spf13/cobra"
)

var ciCmd = &cobra.Command{
	Use:   "ci",
	Short: "Continuous integration helpers",
}

func init() {
	rootCmd.AddCommand(ciCmd)
}
