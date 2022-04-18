package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

const (
	verboseFlag = "verbose"
)

var rootCmd = &cobra.Command{
	Use:   "weron",
	Short: "WebRTC Overlay Networks",
	Long: `Overlay networks based on WebRTC.

Find more information at:
https://github.com/pojntfx/webrtcfd`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		viper.SetEnvPrefix("weron")
		viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))

		if err := viper.BindPFlags(cmd.PersistentFlags()); err != nil {
			return err
		}

		if verbose := viper.GetBool(verboseFlag); verbose {
			boil.DebugMode = true
		}

		return nil
	},
}

func Execute() error {
	rootCmd.PersistentFlags().BoolP(verboseFlag, "v", false, "Enable verbose logging")

	if err := viper.BindPFlags(rootCmd.PersistentFlags()); err != nil {
		return err
	}

	viper.AutomaticEnv()

	return rootCmd.Execute()
}
