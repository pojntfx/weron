package cmd

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/pojntfx/weron/pkg/wrtcconn"
	"github.com/pojntfx/weron/pkg/wrtcthr"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/teivah/broadcast"
)

const (
	serverFlag       = "server"
	packetLengthFlag = "packet-length"
	packetCountFlag  = "packet-count"
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

		adapter := wrtcthr.NewAdapter(
			u.String(),
			viper.GetString(keyFlag),
			viper.GetStringSlice(iceFlag),
			&wrtcthr.AdapterConfig{
				OnSignalerConnect: func(s string) {
					log.Println("Connected to signaler as", s)
				},
				OnPeerConnect: func(s string) {
					log.Println("Connected to peer", s)
				},
				OnPeerDisconnected: func(s string) {
					log.Println("Disconnected from peer", s)
				},
				AdapterConfig: &wrtcconn.AdapterConfig{
					Timeout:    viper.GetDuration(timeoutFlag),
					Verbose:    viper.GetBool(verboseFlag),
					ForceRelay: viper.GetBool(forceRelayFlag),
				},
				Server:       viper.GetBool(serverFlag),
				PacketLength: viper.GetInt(packetLengthFlag),
				PacketCount:  viper.GetInt(packetCountFlag),
			},
			ctx,
		)

		acked := false
		totaled := broadcast.NewRelay[struct{}]()

		go func() {
			for {
				select {
				case <-ctx.Done():
					if err := ctx.Err(); err != context.Canceled {
						panic(err)
					}

					return
				case ack := <-adapter.Acknowledgement():
					log.Printf(
						"%.3f MB/s (%.3f Mb/s) (%v MB read in %v)",
						ack.ThroughputMB,
						ack.ThroughputMb,
						ack.TransferredMB,
						ack.TransferredDuration,
					)

					acked = true
				case totals := <-adapter.Totals():
					fmt.Printf(
						"Average throughput: %.3f MB/s (%.3f Mb/s) (%v MB written in %v) Min: %.3f MB/s Max: %.3f MB/s\n",
						totals.ThroughputAverageMB,
						totals.ThroughputAverageMb,
						totals.TransferredMB,
						totals.TransferredDuration,
						totals.ThroughputMin,
						totals.ThroughputMax,
					)

					totaled.Broadcast(struct{}{})
				}
			}
		}()

		log.Println("Connecting to signaler", viper.GetString(raddrFlag))

		if err := adapter.Open(); err != nil {
			return err
		}
		addInterruptHandler(
			cancel,
			adapter,
			viper.GetBool(verboseFlag),
			func() {
				if !viper.GetBool(serverFlag) && acked {
					l := totaled.Listener(0)
					defer l.Close()

					adapter.GatherTotals()

					<-l.Ch()
				}
			},
		)

		return adapter.Wait()
	},
}

func init() {
	utilityThroughputCmd.PersistentFlags().String(raddrFlag, "wss://weron.herokuapp.com/", "Remote address")
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
