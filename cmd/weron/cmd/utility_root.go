package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var utilityCmd = &cobra.Command{
	Use:     "utility",
	Aliases: []string{"uti", "u"},
	Short:   "Utilities for overlay networks",
}

func init() {
	viper.AutomaticEnv()

	rootCmd.AddCommand(utilityCmd)
}
