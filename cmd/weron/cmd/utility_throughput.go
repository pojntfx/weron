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
	serverFlag       = "server"
	packetLengthFlag = "packet-length"
	packetCountFlag  = "packet-count"

	acklen = 100
)

var utilityThroughputCmd = &cobra.Command{
	Use:     "throughput",
	Aliases: []string{"thr", "t"},
	Short:   "Measure the throughput of the overlay network",
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
			[]string{services.ThroughputPrimary},
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

				totalTransferred := 0
				totalStart := time.Now()

				minSpeed := math.MaxFloat64
				maxSpeed := float64(0)

				printTotals := func() {
					if totalTransferred >= 1 {
						totalDuration := time.Since(totalStart)

						totalSpeed := (float64(totalTransferred) / totalDuration.Seconds()) / 1000000

						fmt.Printf("Average throughput: %.3f MB/s (%.3f Mb/s) (%v MB written in %v) Min: %.3f MB/s Max: %.3f MB/s\n", totalSpeed, totalSpeed*8, totalTransferred/1000000, totalDuration, minSpeed, maxSpeed)
					}
				}

				s := make(chan os.Signal)
				signal.Notify(s, os.Interrupt, syscall.SIGTERM)
				go func() {
					<-s

					printTotals()

					os.Exit(0)
				}()

				if viper.GetBool(serverFlag) {
					go func() {
						defer func() {
							fmt.Printf("\r\u001b[0K-%v@%v\n", peer.PeerID, peer.ChannelID)

							printTotals()
						}()

						for {
							start := time.Now()

							read := 0
							for i := 0; i < viper.GetInt(packetCountFlag); i++ {
								buf := make([]byte, viper.GetInt(packetLengthFlag))

								n, err := peer.Conn.Read(buf)
								if err != nil {
									if viper.GetBool(verboseFlag) {
										log.Println("Could not read from peer, stopping")
									}

									return
								}

								read += n
							}

							if _, err := peer.Conn.Write(make([]byte, acklen)); err != nil {
								if viper.GetBool(verboseFlag) {
									log.Println("Could not write to peer, stopping")
								}

								return
							}

							duration := time.Since(start)

							speed := (float64(read) / duration.Seconds()) / 1000000

							if speed < float64(minSpeed) {
								minSpeed = speed
							}

							if speed > float64(maxSpeed) {
								maxSpeed = speed
							}

							log.Printf("%.3f MB/s (%.3f Mb/s) (%v MB read in %v)", speed, speed*8, read/1000000, duration)

							totalTransferred += read
						}
					}()
				} else {
					go func() {
						defer func() {
							fmt.Printf("\r\u001b[0K-%v@%v\n", peer.PeerID, peer.ChannelID)

							printTotals()
						}()

						for {
							start := time.Now()

							written := 0
							for i := 0; i < viper.GetInt(packetCountFlag); i++ {
								buf := make([]byte, viper.GetInt(packetLengthFlag))
								if _, err := rand.Read(buf); err != nil {
									errs <- err

									return
								}

								n, err := peer.Conn.Write(buf)
								if err != nil {
									if viper.GetBool(verboseFlag) {
										log.Println("Could not write to peer, stopping")
									}

									return
								}

								written += n
							}

							buf := make([]byte, acklen)
							if _, err := peer.Conn.Read(buf); err != nil {
								if viper.GetBool(verboseFlag) {
									log.Println("Could not read from peer, stopping")
								}

								return
							}

							duration := time.Since(start)

							speed := (float64(written) / duration.Seconds()) / 1000000

							if speed < float64(minSpeed) {
								minSpeed = speed
							}

							if speed > float64(maxSpeed) {
								maxSpeed = speed
							}

							log.Printf("%.3f MB/s (%.3f Mb/s) (%v MB written in %v)", speed, speed*8, written/1000000, duration)

							totalTransferred += written
						}
					}()
				}
			}
		}
	},
}

func init() {
	utilityThroughputCmd.PersistentFlags().String(raddrFlag, "wss://webrtcfd.herokuapp.com/", "Remote address")
	utilityThroughputCmd.PersistentFlags().Duration(timeoutFlag, time.Second*10, "Time to wait for connections")
	utilityThroughputCmd.PersistentFlags().String(communityFlag, "", "ID of community to join")
	utilityThroughputCmd.PersistentFlags().String(passwordFlag, "", "Password for community")
	utilityThroughputCmd.PersistentFlags().String(keyFlag, "", "Encryption key for community")
	utilityThroughputCmd.PersistentFlags().StringSlice(iceFlag, []string{"stun:stun.l.google.com:19302"}, "Comma-separated list of STUN servers (in format stun:host:port) and TURN servers to use (in format username:credential@turn:host:port) (i.e. username:credential@turn:global.turn.twilio.com:3478?transport=tcp)")
	utilityThroughputCmd.PersistentFlags().Bool(forceRelayFlag, false, "Force usage of TURN servers")
	utilityThroughputCmd.PersistentFlags().Bool(serverFlag, false, "Act as a server")
	utilityThroughputCmd.PersistentFlags().Int(packetLengthFlag, 50000, "Size of packet to send")
	utilityThroughputCmd.PersistentFlags().Int(packetCountFlag, 1000, "Amount of packets to send before waiting for acknowledgement")

	viper.AutomaticEnv()

	utilityCmd.AddCommand(utilityThroughputCmd)
}
