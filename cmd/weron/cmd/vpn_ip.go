package cmd

import (
	"context"
	"errors"
	"log"
	"net"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/pojntfx/weron/pkg/services"
	"github.com/pojntfx/weron/pkg/wrtcconn"
	"github.com/pojntfx/weron/pkg/wrtcip"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	errMissingIPs  = errors.New("no IP(s) provided")
	errInvalidCIDR = errors.New("invalid CIDR notation for IPs")
)

const (
	ipsFlag        = "ips"
	maxRetriesFlag = "max-retries"
	staticFlag     = "static"
)

var vpnIPCmd = &cobra.Command{
	Use:     "ip",
	Aliases: []string{"i"},
	Short:   "Join a layer 3 overlay network",
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

		if len(viper.GetStringSlice(ipsFlag)) <= 0 {
			return errMissingIPs
		}

		for _, ip := range viper.GetStringSlice(ipsFlag) {
			if _, _, err := net.ParseCIDR(ip); err != nil {
				return errInvalidCIDR
			}
		}

		u, err := url.Parse(viper.GetString(raddrFlag))
		if err != nil {
			return err
		}

		q := u.Query()
		q.Set("community", viper.GetString(communityFlag))
		q.Set("password", viper.GetString(passwordFlag))
		u.RawQuery = q.Encode()

		adapter := wrtcip.NewAdapter(
			u.String(),
			viper.GetString(keyFlag),
			viper.GetStringSlice(iceFlag),
			&wrtcip.AdapterConfig{
				Device: viper.GetString(devFlag),
				OnSignalerConnect: func(s string) {
					log.Println("Connected to signaler as", s)
				},
				OnPeerConnect: func(s string) {
					log.Println("Connected to peer", s)
				},
				OnPeerDisconnected: func(s string) {
					log.Println("Disconnected from peer", s)
				},
				CIDRs:      viper.GetStringSlice(ipsFlag),
				MaxRetries: viper.GetInt(maxRetriesFlag),
				Parallel:   viper.GetInt(parallelFlag),
				NamedAdapterConfig: &wrtcconn.NamedAdapterConfig{
					AdapterConfig: &wrtcconn.AdapterConfig{
						Timeout:    viper.GetDuration(timeoutFlag),
						Verbose:    viper.GetBool(verboseFlag),
						ForceRelay: viper.GetBool(forceRelayFlag),
					},
					IDChannel: viper.GetString(idChannelFlag),
					Kicks:     viper.GetDuration(kicksFlag),
				},
				Static: viper.GetBool(staticFlag),
			},
			ctx,
		)

		log.Println("Connecting to signaler", viper.GetString(raddrFlag))

		if err := adapter.Open(); err != nil {
			return err
		}
		addInterruptHandler(cancel, adapter, viper.GetBool(verboseFlag), nil)

		return adapter.Wait()
	},
}

func init() {
	vpnIPCmd.PersistentFlags().String(raddrFlag, "wss://weron.herokuapp.com/", "Remote address")
	vpnIPCmd.PersistentFlags().Duration(timeoutFlag, time.Second*10, "Time to wait for connections")
	vpnIPCmd.PersistentFlags().String(communityFlag, "", "ID of community to join")
	vpnIPCmd.PersistentFlags().String(passwordFlag, "", "Password for community")
	vpnIPCmd.PersistentFlags().String(keyFlag, "", "Encryption key for community")
	vpnIPCmd.PersistentFlags().StringSlice(iceFlag, []string{"stun:stun.l.google.com:19302"}, "Comma-separated list of STUN servers (in format stun:host:port) and TURN servers to use (in format username:credential@turn:host:port) (i.e. username:credential@turn:global.turn.twilio.com:3478?transport=tcp)")
	vpnIPCmd.PersistentFlags().Bool(forceRelayFlag, false, "Force usage of TURN servers")
	vpnIPCmd.PersistentFlags().String(devFlag, "", "Name to give to the TAP device (i.e. weron0) (default is auto-generated; only supported on Linux, macOS and Windows)")
	vpnIPCmd.PersistentFlags().StringSlice(ipsFlag, []string{""}, "Comma-separated list of IP networks to claim an IP address from and and give to the TUN device (i.e. 2001:db8::1/32,192.0.2.1/24) (on Windows, only one IPv4 and one IPv6 address are supported; on macOS, IPv4 addresses are ignored)")
	vpnIPCmd.PersistentFlags().Bool(staticFlag, false, "Try to claim the exact IPs specified in the --"+ipsFlag+" flag statically instead of selecting a random one from the specified network")
	vpnIPCmd.PersistentFlags().Int(parallelFlag, runtime.NumCPU(), "Amount of threads to use to decode frames")
	vpnIPCmd.PersistentFlags().String(idChannelFlag, services.IPID, "Channel to use to negotiate names")
	vpnIPCmd.PersistentFlags().Duration(kicksFlag, time.Second*5, "Time to wait for kicks")
	vpnIPCmd.PersistentFlags().Int(maxRetriesFlag, 200, "Maximum amount of times to try and claim an IP address")

	viper.AutomaticEnv()

	vpnCmd.AddCommand(vpnIPCmd)
}
