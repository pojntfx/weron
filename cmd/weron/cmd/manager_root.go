package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var managerCmd = &cobra.Command{
	Use:     "manager",
	Aliases: []string{"mgr", "m"},
	Short:   "Manage a signaling server",
}

func init() {
	viper.AutomaticEnv()

	rootCmd.AddCommand(managerCmd)
}
