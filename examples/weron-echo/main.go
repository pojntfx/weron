package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/pojntfx/weron/pkg/wrtcconn"
	"github.com/rs/zerolog"
)

var (
	errMissingCommunity = errors.New("missing community")
	errMissingPassword  = errors.New("missing password")
	errMissingKey       = errors.New("missing key")
)

func main() {
	verboseFlag := flag.Int("verbose", 5, "Verbosity level (0 is disabled, default is info, 7 is trace)")
	raddrFlag := flag.String("raddr", "wss://weron.up.railway.app/", "Remote address")
	timeoutFlag := flag.Duration("timeout", time.Second*10, "Time to wait for connections")
	communityFlag := flag.String("community", "", "ID of community to join")
	passwordFlag := flag.String("password", "", "Password for community")
	keyFlag := flag.String("key", "", "Encryption key for community")
	iceFlag := flag.String("ice", "stun:stun.l.google.com:19302", "Comma-separated list of STUN servers (in format stun:host:port) and TURN servers to use (in format username:credential@turn:host:port) (i.e. username:credential@turn:global.turn.twilio.com:3478?transport=tcp)")
	forceRelayFlag := flag.Bool("force-relay", false, "Force usage of TURN servers")
	pauseFlag := flag.Duration("pause", time.Second*1, "Time to wait before sending next message")

	flag.Parse()

	switch *verboseFlag {
	case 0:
		zerolog.SetGlobalLevel(zerolog.Disabled)
	case 1:
		zerolog.SetGlobalLevel(zerolog.PanicLevel)
	case 2:
		zerolog.SetGlobalLevel(zerolog.FatalLevel)
	case 3:
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	case 4:
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case 5:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case 6:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if strings.TrimSpace(*communityFlag) == "" {
		panic(errMissingCommunity)
	}

	if strings.TrimSpace(*passwordFlag) == "" {
		panic(errMissingPassword)
	}

	if strings.TrimSpace(*keyFlag) == "" {
		panic(errMissingKey)
	}

	log.Println("Connecting to signaler with address", *raddrFlag)

	u, err := url.Parse(*raddrFlag)
	if err != nil {
		panic(err)
	}

	q := u.Query()
	q.Set("community", *communityFlag)
	q.Set("password", *passwordFlag)
	u.RawQuery = q.Encode()

	adapter := wrtcconn.NewAdapter(
		u.String(),
		*keyFlag,
		strings.Split(*iceFlag, ","),
		[]string{"weron/example/echo"},
		&wrtcconn.AdapterConfig{
			Timeout:    *timeoutFlag,
			ForceRelay: *forceRelayFlag,
			OnSignalerReconnect: func() {
				log.Println("Reconnecting to signaler with address", *raddrFlag)
			},
		},
		ctx,
	)

	ids, err := adapter.Open()
	if err != nil {
		panic(err)
	}
	defer adapter.Close()

	id := ""
	errs := make(chan error)
	for {
		select {
		case <-ctx.Done():
			if err := ctx.Err(); err != context.Canceled {
				panic(err)
			}

			return
		case err := <-errs:
			panic(err)
		case rid := <-ids:
			id = rid

			log.Println("Connected to signaler with address", *raddrFlag, "and ID", rid)
		case peer := <-adapter.Accept():
			go func() {
				defer func() {
					log.Println("Disconnected from peer with ID", peer.PeerID, "and channel", peer.ChannelID)
				}()

				log.Println("Connected to peer with ID", peer.PeerID, "and channel", peer.ChannelID)

				go func() {
					ticker := time.NewTicker(*pauseFlag)

					for {
						select {
						case <-ctx.Done():
							ticker.Stop()

							return
						case <-ticker.C:
							if _, err := peer.Conn.Write([]byte(fmt.Sprintf("Hello from %v! It is currently %v local time.\n", id, time.Now().Local()))); err != nil {
								return
							}
						}
					}
				}()

				reader := bufio.NewScanner(peer.Conn)

				for reader.Scan() {
					fmt.Printf("Got message from peer %v: %v\n", peer.PeerID, reader.Text())
				}
			}()
		}
	}
}
