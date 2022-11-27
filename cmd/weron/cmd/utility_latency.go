package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/pojntfx/weron/pkg/wrtcconn"
	"github.com/pojntfx/weron/pkg/wrtcltc"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/teivah/broadcast"
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

		adapter := wrtcltc.NewAdapter(
			u.String(),
			viper.GetString(keyFlag),
			viper.GetStringSlice(iceFlag),
			&wrtcltc.AdapterConfig{
				OnSignalerConnect: func(s string) {
					log.Info().
						Str("id", s).
						Msg("Connected to signaler")
				},
				OnPeerConnect: func(s string) {
					log.Info().
						Str("id", s).
						Msg("Connected to peer")
				},
				OnPeerDisconnected: func(s string) {
					log.Info().
						Str("id", s).
						Msg("Disconnected from peer")
				},
				AdapterConfig: &wrtcconn.AdapterConfig{
					Timeout:    viper.GetDuration(timeoutFlag),
					ForceRelay: viper.GetBool(forceRelayFlag),
				},
				Server:       viper.GetBool(serverFlag),
				PacketLength: viper.GetInt(packetLengthFlag),
				Pause:        viper.GetDuration(pauseFlag),
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
				case ack := <-adapter.Acknowledgements():
					fmt.Printf("%v B written and acknowledged in %v\n", ack.BytesWritten, ack.Latency)

					acked = true
				case totals := <-adapter.Totals():
					fmt.Printf("Average latency: %v (%v packets written) Min: %v Max: %v\n", totals.LatencyAverage, totals.PacketsWritten, totals.LatencyMin, totals.LatencyMax)

					totaled.Broadcast(struct{}{})
				}
			}
		}()

		log.Info().
			Str("addr", viper.GetString(raddrFlag)).
			Msg("Connecting to signaler")

		if err := adapter.Open(); err != nil {
			return err
		}
		addInterruptHandler(
			cancel,
			adapter,
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
	utilityLatencyCommand.PersistentFlags().String(raddrFlag, "wss://weron.up.railway.app/", "Remote address")
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
