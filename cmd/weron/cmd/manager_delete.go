package cmd

import (
	"context"
	"strings"

	"github.com/pojntfx/webrtcfd/pkg/wrtcmgr"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var managerDeleteCmd = &cobra.Command{
	Use:     "delete",
	Aliases: []string{"del", "d", "rm"},
	Short:   "Delete a persistent or ephermal community",
	PreRunE: validateRemoteFlags,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := viper.BindPFlags(cmd.PersistentFlags()); err != nil {
			return err
		}

		if strings.TrimSpace(viper.GetString(apiPasswordFlag)) == "" {
			return errMissingAPIPassword
		}

		if strings.TrimSpace(viper.GetString(apiUsernameFlag)) == "" {
			return errMissingAPIUsername
		}

		if strings.TrimSpace(viper.GetString(communityFlag)) == "" {
			return errMissingCommunity
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		manager := wrtcmgr.NewManager(
			viper.GetString(raddrFlag),
			viper.GetString(apiUsernameFlag),
			viper.GetString(apiPasswordFlag),
			ctx,
		)

		if err := manager.DeleteCommunity(viper.GetString(communityFlag)); err != nil {
			return err
		}

		return nil
	},
}

func init() {
	addRemoteFlags(managerDeleteCmd.PersistentFlags())
	managerDeleteCmd.PersistentFlags().String(communityFlag, "", "ID of community to create")

	viper.AutomaticEnv()

	managerCmd.AddCommand(managerDeleteCmd)
}
