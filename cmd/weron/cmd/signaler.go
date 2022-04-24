package cmd

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/pojntfx/weron/pkg/wrtcsgl"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	laddrFlag               = "laddr"
	heartbeatFlag           = "heartbeat"
	postgresURLFlag         = "postgres-url"
	redisURLFlag            = "redis-url"
	cleanupFlag             = "cleanup"
	ephermalCommunitiesFlag = "ephermal-communities"
	apiUsernameFlag         = "api-username"
	apiPasswordFlag         = "api-password"
	oidcIssuerFlag          = "oidc-issuer"
	oidcClientIDFlag        = "oidc-client-id"
)

var signalerCmd = &cobra.Command{
	Use:     "signaler",
	Aliases: []string{"sgl", "s"},
	Short:   "Start a signaling server",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if u := os.Getenv("API_USERNAME"); u != "" {
			if viper.GetBool(verboseFlag) {
				log.Println("Using username from API_USERNAME env variable")
			}

			viper.Set(apiUsernameFlag, u)
		}

		if u := os.Getenv("API_PASSWORD"); u != "" {
			if viper.GetBool(verboseFlag) {
				log.Println("Using password from API_PASSWORD env variable")
			}

			viper.Set(apiPasswordFlag, u)
		}

		if u := os.Getenv("DATABASE_URL"); u != "" {
			if viper.GetBool(verboseFlag) {
				log.Println("Using database URL from DATABASE_URL env variable")
			}

			viper.Set(postgresURLFlag, u)
		}

		if u := os.Getenv("REDIS_URL"); u != "" {
			if viper.GetBool(verboseFlag) {
				log.Println("Using broker URL from REDIS_URL env variable")
			}

			viper.Set(redisURLFlag, u)
		}

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

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		addr, err := net.ResolveTCPAddr("tcp", viper.GetString(laddrFlag))
		if err != nil {
			return err
		}

		if port := os.Getenv("PORT"); port != "" {
			if viper.GetBool(verboseFlag) {
				log.Println("Using port from PORT env variable")
			}

			p, err := strconv.Atoi(port)
			if err != nil {
				return err
			}

			addr.Port = p
		}

		signaler := wrtcsgl.NewSignaler(
			addr.String(),
			viper.GetString(postgresURLFlag),
			viper.GetString(redisURLFlag),
			&wrtcsgl.SignalerConfig{
				Heartbeat:           viper.GetDuration(heartbeatFlag),
				Cleanup:             viper.GetBool(cleanupFlag),
				EphermalCommunities: viper.GetBool(ephermalCommunitiesFlag),
				APIUsername:         viper.GetString(apiUsernameFlag),
				APIPassword:         viper.GetString(apiPasswordFlag),
				OIDCIssuer:          viper.GetString(oidcIssuerFlag),
				OIDCClientID:        viper.GetString(oidcClientIDFlag),
				Verbose:             viper.GetBool(verboseFlag),
				OnConnect: func(raddr, community string) {
					log.Println("Connected to client with address", raddr, "in community", community)
				},
				OnDisconnect: func(raddr, community string, err interface{}) {
					log.Println("Disconnected from client with address", raddr, "in community", community)
				},
			},
			ctx,
		)

		if err := signaler.Open(); err != nil {
			return err
		}

		s := make(chan os.Signal)
		signal.Notify(s, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-s

			log.Println("Gracefully shutting down signaling server")

			go func() {
				<-s

				log.Println("Forcing shutdown of signaling server")

				cancel()

				if err := signaler.Close(); err != nil {
					panic(err)
				}
			}()

			if err := signaler.Close(); err != nil {
				panic(err)
			}

			cancel()
		}()

		defer func() {
			if err := signaler.Close(); err != nil {
				panic(err)
			}
		}()

		log.Println("Listening on address", addr.String())

		return signaler.Wait()
	},
}

func init() {
	signalerCmd.PersistentFlags().String(laddrFlag, ":1337", "Listening address (can also be set using the PORT env variable)")
	signalerCmd.PersistentFlags().Duration(heartbeatFlag, time.Second*10, "Time to wait for heartbeats")
	signalerCmd.PersistentFlags().String(postgresURLFlag, "", "URL of PostgreSQL database to use (i.e. postgres://myuser:mypassword@myhost:myport/mydatabase) (can also be set using the DATABASE_URL env variable). If empty, a in-memory database will be used.")
	signalerCmd.PersistentFlags().String(redisURLFlag, "", "URL of Redis database to use (i.e. redis://myuser:mypassword@localhost:6379/1) (can also be set using the REDIS_URL env variable). If empty, a in-process broker will be used.")
	signalerCmd.PersistentFlags().Bool(cleanupFlag, false, "(Warning: Only enable this after stopping all other servers accessing the database!) Remove all ephermal communities from database and reset client counts before starting")
	signalerCmd.PersistentFlags().Bool(ephermalCommunitiesFlag, true, "Enable the creation of ephermal communities")
	signalerCmd.PersistentFlags().String(apiUsernameFlag, "admin", "Username for the management API (can also be set using the API_USERNAME env variable). Ignored if any of the OIDC parameters are set.")
	signalerCmd.PersistentFlags().String(apiPasswordFlag, "", "Password for the management API (can also be set using the API_PASSWORD env variable). Ignored if any of the OIDC parameters are set.")
	signalerCmd.PersistentFlags().String(oidcIssuerFlag, "", "OIDC Issuer (i.e. https://pojntfx.eu.auth0.com/) (can also be set using the OIDC_ISSUER env variable)")
	signalerCmd.PersistentFlags().String(oidcClientIDFlag, "", "OIDC Client ID (i.e. myoidcclientid) (can also be set using the OIDC_CLIENT_ID env variable)")

	viper.AutomaticEnv()

	rootCmd.AddCommand(signalerCmd)
}
