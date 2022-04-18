package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/pojntfx/webrtcfd/pkg/wrtcconn"
	"github.com/teivah/broadcast"
)

var (
	errMissingCommunity = errors.New("missing community")
	errMissingPassword  = errors.New("missing password")

	errMissingKey       = errors.New("missing key")
	errMissingUsernames = errors.New("missing usernames")
)

func main() {
	raddr := flag.String("raddr", "wss://webrtcfd.herokuapp.com/", "Remote address")
	timeout := flag.Duration("timeout", time.Second*10, "Time to wait for connections")
	community := flag.String("community", "", "ID of community to join")
	password := flag.String("password", "", "Password for community")
	key := flag.String("key", "", "Encryption key for community")
	names := flag.String("names", "", "Comma-seperated list of names to try and claim one from")
	channel := flag.String("channel", "wrtcid.primary", "Comma-seperated list of channels in community to join")
	idChannel := flag.String("id-channel", "wrtcid.id", "Channel to use to negotiate names")
	ice := flag.String("ice", "stun:stun.l.google.com:19302", "Comma-seperated list of STUN servers (in format stun:host:port) and TURN servers to use (in format username:credential@turn:host:port) (i.e. username:credential@turn:global.turn.twilio.com:3478?transport=tcp)")
	relay := flag.Bool("force-relay", false, "Force usage of TURN servers")
	kicks := flag.Duration("kicks", time.Second*5, "Time to wait for kicks")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")

	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if strings.TrimSpace(*community) == "" {
		panic(errMissingCommunity)
	}

	if strings.TrimSpace(*password) == "" {
		panic(errMissingPassword)
	}

	if strings.TrimSpace(*key) == "" {
		panic(errMissingKey)
	}

	if strings.TrimSpace(*names) == "" {
		panic(errMissingUsernames)
	}

	fmt.Printf(".%v\n", *raddr)

	u, err := url.Parse(*raddr)
	if err != nil {
		panic(err)
	}

	q := u.Query()
	q.Set("community", *community)
	q.Set("password", *password)
	u.RawQuery = q.Encode()

	adapter := wrtcconn.NewNamedAdapter(
		u.String(),
		*key,
		strings.Split(*ice, ","),
		strings.Split(*channel, ","),
		&wrtcconn.NamedAdapterConfig{
			AdapterConfig: &wrtcconn.AdapterConfig{
				Timeout:    *timeout,
				Verbose:    *verbose,
				ForceRelay: *relay,
			},
			IDChannel: *idChannel,
			Names:     strings.Split(*names, ","),
			Kicks:     *kicks,
		},
		ctx,
	)

	ids, err := adapter.Open()
	if err != nil {
		panic(err)
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
			return
		case id = <-ids:
			fmt.Printf("\r\u001b[0K%v.", id)
		case err = <-adapter.Err():
			panic(err)
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
}
