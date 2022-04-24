package cmd

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"math"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pojntfx/webrtcfd/pkg/services"
	"github.com/pojntfx/webrtcfd/pkg/wrtcconn"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	pauseFlag = "pause"
)

var utilityLatencyCommand = &cobra.Command{
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
								if viper.GetBool(verboseFlag) {
									log.Println("Could not read from peer, stopping")
								}

								return
							}

							if _, err := peer.Conn.Write(buf); err != nil {
								if viper.GetBool(verboseFlag) {
									log.Println("Could not write to peer, stopping")
								}

								return
							}
						}
					}()
				} else {
					go func() {
						packetsWritten := int64(0)
						totalLatency := time.Duration(0)

						minLatency := time.Duration(math.MaxInt64)
						maxLatency := time.Duration(0)

						printTotals := func() {
							if packetsWritten >= 1 {
								averageLatency := totalLatency.Nanoseconds() / packetsWritten

								fmt.Printf("Average latency: %v (%v packets written) Min: %v Max: %v\n", time.Duration(averageLatency), packetsWritten, minLatency, maxLatency)
							}
						}

						s := make(chan os.Signal)
						signal.Notify(s, os.Interrupt, syscall.SIGTERM)
						go func() {
							<-s

							printTotals()

							os.Exit(0)
						}()

						defer func() {
							fmt.Printf("\r\u001b[0K-%v@%v\n", peer.PeerID, peer.ChannelID)

							printTotals()
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
								if viper.GetBool(verboseFlag) {
									log.Println("Could not write to peer, stopping")
								}

								return
							}

							if _, err := peer.Conn.Read(buf); err != nil {
								if viper.GetBool(verboseFlag) {
									log.Println("Could not read from peer, stopping")
								}

								return
							}

							latency := time.Since(start)

							if latency < minLatency {
								minLatency = latency
							}

							if latency > maxLatency {
								maxLatency = latency
							}

							totalLatency += latency
							packetsWritten++

							log.Printf("%v B written and acknowledged in %v", written, latency)

							time.Sleep(viper.GetDuration(pauseFlag))
						}
					}()
				}
			}
		}
	},
}

func init() {
	utilityLatencyCommand.PersistentFlags().String(raddrFlag, "wss://webrtcfd.herokuapp.com/", "Remote address")
	utilityLatencyCommand.PersistentFlags().Duration(timeoutFlag, time.Second*10, "Time to wait for connections")
	utilityLatencyCommand.PersistentFlags().String(communityFlag, "", "ID of community to join")
	utilityLatencyCommand.PersistentFlags().String(passwordFlag, "", "Password for community")
	utilityLatencyCommand.PersistentFlags().String(keyFlag, "", "Encryption key for community")
	utilityLatencyCommand.PersistentFlags().StringSlice(iceFlag, []string{"stun:stun.l.google.com:19302"}, "Comma-separated list of STUN servers (in format stun:host:port) and TURN servers to use (in format username:credential@turn:host:port) (i.e. username:credential@turn:global.turn.twilio.com:3478?transport=tcp)")
	utilityLatencyCommand.PersistentFlags().Bool(forceRelayFlag, false, "Force usage of TURN servers")
	utilityLatencyCommand.PersistentFlags().Bool(serverFlag, false, "Act as a server")
	utilityLatencyCommand.PersistentFlags().Int(packetLengthFlag, 128, "Size of packet to send and acknowledge")
	utilityLatencyCommand.PersistentFlags().Duration(pauseFlag, time.Second*1, "Time to wait before sending next packet")

	viper.AutomaticEnv()

	utilityCmd.AddCommand(utilityLatencyCommand)
}
