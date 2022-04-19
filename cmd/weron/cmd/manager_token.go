package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/pkg/browser"
	"github.com/pojntfx/webrtcfd/pkg/wrtctkn"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	errMissingOIDCIssuer      = errors.New("missing OIDC issuer")
	errMissingOIDCClientID    = errors.New("missing OIDC client ID")
	errMissingOIDCRedirectURL = errors.New("missing OIDC redirect URL")
)

const (
	oidcRedirectURLFlag = "oidc-redirect-url"
)

var managerTokenCmd = &cobra.Command{
	Use:     "token",
	Aliases: []string{"tkn", "t"},
	Short:   "Generate a OIDC token",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if u := os.Getenv("OIDC_ISSUER"); u != "" {
			if viper.GetBool(verboseFlag) {
				log.Println("Using OIDC issuer from OIDC_ISSUER env variable")
			}

			viper.Set(oidcIssuerFlag, u)
		}

		if u := os.Getenv("OIDC_CLIENT_ID"); u != "" {
			if viper.GetBool(verboseFlag) {
				log.Println("Using OIDC client ID from OIDC_CLIENT_ID env variable")
			}

			viper.Set(oidcClientIDFlag, u)
		}

		return viper.BindPFlags(cmd.PersistentFlags())
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := viper.BindPFlags(cmd.PersistentFlags()); err != nil {
			return err
		}

		if strings.TrimSpace(viper.GetString(oidcIssuerFlag)) == "" {
			return errMissingOIDCIssuer
		}

		if strings.TrimSpace(viper.GetString(oidcClientIDFlag)) == "" {
			return errMissingOIDCClientID
		}

		if strings.TrimSpace(viper.GetString(oidcRedirectURLFlag)) == "" {
			return errMissingOIDCRedirectURL
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		manager := wrtctkn.NewTokenManager(
			viper.GetString(oidcIssuerFlag),
			viper.GetString(oidcClientIDFlag),
			viper.GetString(oidcRedirectURLFlag),

			func(s string) error {
				if err := browser.OpenURL(s); err != nil {
					log.Printf(`Could not open browser, please open the following URL in your browser manually to authorize:
%v`, s)
				}

				return nil
			},

			ctx,
		)

		token, err := manager.GetToken()
		if err != nil {
			return err
		}

		log.Println("Successfully got the following OIDC access token:")

		fmt.Println(token)

		return nil
	},
}

func init() {
	managerTokenCmd.PersistentFlags().String(oidcIssuerFlag, "", "OIDC Issuer (i.e. https://pojntfx.eu.auth0.com/) (can also be set using the OIDC_ISSUER env variable)")
	managerTokenCmd.PersistentFlags().String(oidcClientIDFlag, "", "OIDC Client ID (i.e. myoidcclientid) (can also be set using the OIDC_CLIENT_ID env variable)")
	managerTokenCmd.PersistentFlags().String(oidcRedirectURLFlag, "http://localhost:11337", "OIDC redirect URL")

	viper.AutomaticEnv()

	managerCmd.AddCommand(managerTokenCmd)
}
