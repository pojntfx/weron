package cmd

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/pojntfx/webrtcfd/pkg/services"
	"github.com/pojntfx/webrtcfd/pkg/wrtcconn"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	pauseFlag = "pause"
)

var latencyCmd = &cobra.Command{
	Use:     "latency",
	Aliases: []string{"ltc", "l"},
	Short:   "Measure the latency of the overlay network",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := viper.BindPFlags(cmd.PersistentFlags()); err != nil {
			return err
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if strings.TrimSpace(viper.GetString(communityFlag)) == "" {
			return errMissingCommunity
		}

		if strings.TrimSpace(viper.GetString(passwordFlag)) == "" {
			return errMissingPassword
		}

		if strings.TrimSpace(viper.GetString(keyFlag)) == "" {
			return errMissingKey
		}

		fmt.Printf("\r\u001b[0K.%v\n", viper.GetString(raddrFlag))

		u, err := url.Parse(viper.GetString(raddrFlag))
		if err != nil {
			return err
		}

		q := u.Query()
		q.Set("community", viper.GetString(communityFlag))
		q.Set("password", viper.GetString(passwordFlag))
		u.RawQuery = q.Encode()

		adapter := wrtcconn.NewAdapter(
			u.String(),
			viper.GetString(keyFlag),
			viper.GetStringSlice(iceFlag),
			[]string{services.LatencyPrimary},
			&wrtcconn.AdapterConfig{
				Timeout:    viper.GetDuration(timeoutFlag),
				Verbose:    viper.GetBool(verboseFlag),
				ForceRelay: viper.GetBool(forceRelayFlag),
			},
			ctx,
		)

		ids, err := adapter.Open()
		if err != nil {
			return err
		}
		defer func() {
			if err := adapter.Close(); err != nil {
				panic(err)
			}
		}()

		errs := make(chan error)
		id := ""
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case err := <-errs:
				panic(err)
			case id = <-ids:
				fmt.Printf("\r\u001b[0K%v.", id)
			case peer := <-adapter.Accept():
				fmt.Printf("\r\u001b[0K+%v@%v\n", peer.PeerID, peer.ChannelID)

				if viper.GetBool(serverFlag) {
					go func() {
						defer func() {
							fmt.Printf("\r\u001b[0K-%v@%v\n", peer.PeerID, peer.ChannelID)
						}()

						for {
							buf := make([]byte, viper.GetInt(packetLengthFlag))
							if _, err := peer.Conn.Read(buf); err != nil {
								errs <- err

								return
							}

							if _, err := peer.Conn.Write(buf); err != nil {
								errs <- err

								return
							}
						}
					}()
				} else {
					go func() {
						defer func() {
							fmt.Printf("\r\u001b[0K-%v@%v\n", peer.PeerID, peer.ChannelID)
						}()

						for {
							start := time.Now()

							buf := make([]byte, viper.GetInt(packetLengthFlag))
							if _, err := rand.Read(buf); err != nil {
								errs <- err

								return
							}

							written, err := peer.Conn.Write(buf)
							if err != nil {
								errs <- err

								return
							}

							if _, err := peer.Conn.Read(buf); err != nil {
								errs <- err

								return
							}

							duration := time.Since(start)

							log.Printf("%v B written and acknowledged in %v", written, duration)

							time.Sleep(viper.GetDuration(pauseFlag))
						}
					}()
				}
			}
		}
	},
}

func init() {
	latencyCmd.PersistentFlags().String(raddrFlag, "wss://webrtcfd.herokuapp.com/", "Remote address")
	latencyCmd.PersistentFlags().Duration(timeoutFlag, time.Second*10, "Time to wait for connections")
	latencyCmd.PersistentFlags().String(communityFlag, "", "ID of community to join")
	latencyCmd.PersistentFlags().String(passwordFlag, "", "Password for community")
	latencyCmd.PersistentFlags().String(keyFlag, "", "Encryption key for community")
	latencyCmd.PersistentFlags().StringSlice(iceFlag, []string{"stun:stun.l.google.com:19302"}, "Comma-separated list of STUN servers (in format stun:host:port) and TURN servers to use (in format username:credential@turn:host:port) (i.e. username:credential@turn:global.turn.twilio.com:3478?transport=tcp)")
	latencyCmd.PersistentFlags().Bool(forceRelayFlag, false, "Force usage of TURN servers")
	latencyCmd.PersistentFlags().Bool(serverFlag, false, "Act as a server")
	latencyCmd.PersistentFlags().Int(packetLengthFlag, 128, "Size of packet to send and acknowledge")
	latencyCmd.PersistentFlags().Duration(pauseFlag, time.Second*1, "Time to wait before sending next packet")

	viper.AutomaticEnv()

	rootCmd.AddCommand(latencyCmd)
}
