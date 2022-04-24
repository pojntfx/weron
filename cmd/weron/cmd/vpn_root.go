package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var vpnCmd = &cobra.Command{
	Use:     "vpn",
	Aliases: []string{"vpn", "v"},
	Short:   "Join virtual private networks built on overlay networks",
}

func init() {
	viper.AutomaticEnv()

	rootCmd.AddCommand(vpnCmd)
}
