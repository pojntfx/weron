package cmd

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/pojntfx/webrtcfd/pkg/wrtcmgr"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	errMissingCommunity = errors.New("missing community")
	errMissingPassword  = errors.New("missing password")
)

const (
	communityFlag = "community"
	passwordFlag  = "password"
)

var managerCreateCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"ctr", "c", "mk"},
	Short:   "Create a persistent community",
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

		if strings.TrimSpace(viper.GetString(passwordFlag)) == "" {
			return errMissingPassword
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		manager := wrtcmgr.NewManager(
			viper.GetString(raddrFlag),
			viper.GetString(apiUsernameFlag),
			viper.GetString(apiPasswordFlag),
			ctx,
		)

		c, err := manager.CreatePersistentCommunity(viper.GetString(communityFlag), viper.GetString(passwordFlag))
		if err != nil {
			return err
		}

		w := csv.NewWriter(os.Stdout)
		defer w.Flush()

		if err := w.Write([]string{"id", "clients", "persistent"}); err != nil {
			return err
		}

		return w.Write([]string{c.ID, fmt.Sprintf("%v", c.Clients), fmt.Sprintf("%v", c.Persistent)})
	},
}

func init() {
	addRemoteFlags(managerCreateCmd.PersistentFlags())
	managerCreateCmd.PersistentFlags().String(communityFlag, "", "ID of community to create")
	managerCreateCmd.PersistentFlags().String(passwordFlag, "", "Password for community")

	viper.AutomaticEnv()

	managerCmd.AddCommand(managerCreateCmd)
}
