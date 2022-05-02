package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/pojntfx/weron/pkg/services"
	"github.com/pojntfx/weron/pkg/wrtcchat"
	"github.com/pojntfx/weron/pkg/wrtcconn"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

func addInterruptHandler(cancel func(), closer io.Closer, before func()) {
	s := make(chan os.Signal)
	signal.Notify(s, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-s

		if before != nil {
			before()
		}

		log.Debug().Msg("Gracefully shutting down")

		go func() {
			<-s

			log.Debug().Msg("Forcing shutdown")

			cancel()

			os.Exit(1)
		}()

		if err := closer.Close(); err != nil {
			panic(err)
		}

		cancel()
	}()
}

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

		id := ""
		adapter := wrtcchat.NewAdapter(
			u.String(),
			viper.GetString(keyFlag),
			viper.GetStringSlice(iceFlag),
			&wrtcchat.AdapterConfig{
				OnSignalerConnect: func(s string) {
					id = s

					fmt.Printf("\n%v!\n", id)
				},
				OnPeerConnect: func(peerID, channelID string) {
					fmt.Printf("\r\u001b[0K+%v@%v\n", peerID, channelID)
					fmt.Printf("\r\u001b[0K%v> ", id)
				},
				OnPeerDisconnected: func(peerID, channelID string) {
					fmt.Printf("\r\u001b[0K-%v@%v\n", peerID, channelID)
					fmt.Printf("\r\u001b[0K%v> ", id)
				},
				OnMessage: func(m wrtcchat.Message) {
					fmt.Printf("\r\u001b[0K%v@%v: %s\n", m.PeerID, m.ChannelID, m.Body)
					fmt.Printf("\r\u001b[0K%v> ", id)
				},
				Channels: viper.GetStringSlice(channelsFlag),
				NamedAdapterConfig: &wrtcconn.NamedAdapterConfig{
					AdapterConfig: &wrtcconn.AdapterConfig{
						Timeout:    viper.GetDuration(timeoutFlag),
						ForceRelay: viper.GetBool(forceRelayFlag),
					},
					IDChannel: viper.GetString(idChannelFlag),
					Names:     viper.GetStringSlice(namesFlag),
					Kicks:     viper.GetDuration(kicksFlag),
				},
			},
			ctx,
		)

		if err := adapter.Open(); err != nil {
			return err
		}
		addInterruptHandler(cancel, adapter, nil)

		go func() {
			reader := bufio.NewScanner(os.Stdin)

			for reader.Scan() {
				adapter.SendMessage([]byte(reader.Text() + "\n"))
				fmt.Printf("\r\u001b[0K%v> ", id)
			}
		}()

		return adapter.Wait()
	},
}

func init() {
	chatCmd.PersistentFlags().String(raddrFlag, "wss://weron.herokuapp.com/", "Remote address")
	chatCmd.PersistentFlags().Duration(timeoutFlag, time.Second*10, "Time to wait for connections")
	chatCmd.PersistentFlags().String(communityFlag, "", "ID of community to join")
	chatCmd.PersistentFlags().String(passwordFlag, "", "Password for community")
	chatCmd.PersistentFlags().String(keyFlag, "", "Encryption key for community")
	chatCmd.PersistentFlags().StringSlice(namesFlag, []string{}, "Comma-separated list of names to try and claim one from")
	chatCmd.PersistentFlags().StringSlice(channelsFlag, []string{services.ChatPrimary}, "Comma-separated list of channels in community to join")
	chatCmd.PersistentFlags().String(idChannelFlag, services.ChatID, "Channel to use to negotiate names")
	chatCmd.PersistentFlags().StringSlice(iceFlag, []string{"stun:stun.l.google.com:19302"}, "Comma-separated list of STUN servers (in format stun:host:port) and TURN servers to use (in format username:credential@turn:host:port) (i.e. username:credential@turn:global.turn.twilio.com:3478?transport=tcp)")
	chatCmd.PersistentFlags().Bool(forceRelayFlag, false, "Force usage of TURN servers")
	chatCmd.PersistentFlags().Duration(kicksFlag, time.Second*5, "Time to wait for kicks")

	viper.AutomaticEnv()

	rootCmd.AddCommand(chatCmd)
}
