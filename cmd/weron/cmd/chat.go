package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/pojntfx/webrtcfd/pkg/wrtcconn"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/teivah/broadcast"
)

const (
	timeoutFlag    = "timeout"
	keyFlag        = "key"
	namesFlag      = "names"
	channelsFlag   = "channels"
	idChannelFlag  = "id-channel"
	iceFlag        = "ice"
	forceRelayFlag = "force-relay"
	kicksFlag      = "kicks"
)

var (
	errMissingKey       = errors.New("missing key")
	errMissingUsernames = errors.New("missing usernames")
)

var chatCmd = &cobra.Command{
	Use:     "chat",
	Aliases: []string{"cht", "c"},
	Short:   "Chat over the overlay network",
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

		if len(viper.GetStringSlice(namesFlag)) <= 0 {
			return errMissingUsernames
		}

		fmt.Printf(".%v", viper.GetString(raddrFlag))

		u, err := url.Parse(viper.GetString(raddrFlag))
		if err != nil {
			return err
		}

		q := u.Query()
		q.Set("community", viper.GetString(communityFlag))
		q.Set("password", viper.GetString(passwordFlag))
		u.RawQuery = q.Encode()

		adapter := wrtcconn.NewNamedAdapter(
			u.String(),
			viper.GetString(keyFlag),
			viper.GetStringSlice(iceFlag),
			viper.GetStringSlice(channelsFlag),
			&wrtcconn.NamedAdapterConfig{
				AdapterConfig: &wrtcconn.AdapterConfig{
					Timeout:    viper.GetDuration(timeoutFlag),
					Verbose:    viper.GetBool(verboseFlag),
					ForceRelay: viper.GetBool(forceRelayFlag),
				},
				IDChannel: viper.GetString(idChannelFlag),
				Names:     viper.GetStringSlice(namesFlag),
				Kicks:     viper.GetDuration(kicksFlag),
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

		lines := broadcast.NewRelay[[]byte]()
		go func() {
			reader := bufio.NewScanner(os.Stdin)

			for reader.Scan() {
				lines.Broadcast([]byte(reader.Text() + "\n"))
			}
		}()

		id := ""
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case id = <-ids:
				fmt.Printf("\n%v!\n", id)
			case err = <-adapter.Err():
				return err
			case peer := <-adapter.Accept():
				fmt.Printf("\r\u001b[0K+%v@%v\n", peer.PeerID, peer.ChannelID)
				fmt.Printf("\r\u001b[0K%v@%v> ", id, peer.ChannelID)

				l := lines.Listener(0)

				go func() {
					defer func() {
						fmt.Printf("\r\u001b[0K-%v@%v\n", peer.PeerID, peer.ChannelID)
						fmt.Printf("\r\u001b[0K%v@%v> ", id, peer.ChannelID)

						l.Close()
					}()

					reader := bufio.NewScanner(peer.Conn)
					for reader.Scan() {
						fmt.Printf("\r\u001b[0K%v@%v: %v\n", peer.PeerID, peer.ChannelID, reader.Text())
						fmt.Printf("\r\u001b[0K%v@%v> ", id, peer.ChannelID)
					}
				}()

				go func() {
					for msg := range l.Ch() {
						if _, err := peer.Conn.Write(msg); err != nil {
							return
						}

						fmt.Printf("\r\u001b[0K%v@%v> ", id, peer.ChannelID)
					}
				}()
			}
		}
	},
}

func init() {
	chatCmd.PersistentFlags().String(raddrFlag, "wss://webrtcfd.herokuapp.com/", "Remote address")
	chatCmd.PersistentFlags().Duration(timeoutFlag, time.Second*10, "Time to wait for connections")
	chatCmd.PersistentFlags().String(communityFlag, "", "ID of community to join")
	chatCmd.PersistentFlags().String(passwordFlag, "", "Password for community")
	chatCmd.PersistentFlags().String(keyFlag, "", "Encryption key for community")
	chatCmd.PersistentFlags().StringSlice(namesFlag, []string{}, "Comma-separated list of names to try and claim one from")
	chatCmd.PersistentFlags().StringSlice(channelsFlag, []string{"wrtcid.primary"}, "Comma-separated list of channels in community to join")
	chatCmd.PersistentFlags().String(idChannelFlag, "wrtcid.id", "Channel to use to negotiate names")
	chatCmd.PersistentFlags().StringSlice(iceFlag, []string{"stun:stun.l.google.com:19302"}, "Comma-separated list of STUN servers (in format stun:host:port) and TURN servers to use (in format username:credential@turn:host:port) (i.e. username:credential@turn:global.turn.twilio.com:3478?transport=tcp)")
	chatCmd.PersistentFlags().Bool(forceRelayFlag, false, "Force usage of TURN servers")
	chatCmd.PersistentFlags().Duration(kicksFlag, time.Second*5, "Time to wait for kicks")

	viper.AutomaticEnv()

	rootCmd.AddCommand(chatCmd)
}
