package cmd

import (
	"strings"

	"github.com/rs/zerolog"
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
	Long: `Lean, fast & secure overlay networks based on WebRTC.


Find more information at:
https://github.com/pojntfx/weron`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		viper.SetEnvPrefix("weron")
		viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))

		if err := viper.BindPFlags(cmd.PersistentFlags()); err != nil {
			return err
		}

		verbose := viper.GetInt(verboseFlag)
		if verbose > 5 {
			boil.DebugMode = true
		}

		switch verbose {
		case 0:
			zerolog.SetGlobalLevel(zerolog.Disabled)
		case 1:
			zerolog.SetGlobalLevel(zerolog.PanicLevel)
		case 2:
			zerolog.SetGlobalLevel(zerolog.FatalLevel)
		case 3:
			zerolog.SetGlobalLevel(zerolog.ErrorLevel)
		case 4:
			zerolog.SetGlobalLevel(zerolog.WarnLevel)
		case 5:
			zerolog.SetGlobalLevel(zerolog.InfoLevel)
		case 6:
			zerolog.SetGlobalLevel(zerolog.DebugLevel)
		default:
			zerolog.SetGlobalLevel(zerolog.TraceLevel)
		}

		return nil
	},
}

func Execute() error {
	rootCmd.PersistentFlags().IntP(verboseFlag, "v", 5, "Verbosity level (0 is disabled, default is info, 7 is trace)")

	if err := viper.BindPFlags(rootCmd.PersistentFlags()); err != nil {
		return err
	}

	viper.AutomaticEnv()

	return rootCmd.Execute()
}
