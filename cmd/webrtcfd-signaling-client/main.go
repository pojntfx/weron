package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/pojntfx/webrtcfd/pkg/wrtcconn"
)

var (
	errMissingCommunity = errors.New("missing community")
	errMissingPassword  = errors.New("missing password")

	errMissingKey = errors.New("missing key")

	errInvalidTURNServerAddr  = errors.New("invalid TURN server address")
	errMissingTURNCredentials = errors.New("missing TURN server credentials")
)

const (
	dataChannelName = "webrtcfd"
)

type peer struct {
	conn       *webrtc.PeerConnection
	candidates chan webrtc.ICECandidateInit
	channel    *webrtc.DataChannel
}

func main() {
	raddr := flag.String("raddr", "wss://webrtcfd.herokuapp.com/", "Remote address")
	timeout := flag.Duration("timeout", time.Second*10, "Time to wait for connections")
	community := flag.String("community", "", "ID of community to join")
	password := flag.String("password", "", "Password for community")
	key := flag.String("key", "", "Encryption key for community")
	ice := flag.String("ice", "stun:stun.l.google.com:19302", "Comma-seperated list of STUN servers (in format stun:host:port) and TURN servers to use (in format username:credential@turn:host:port) (i.e. username:credential@turn:global.turn.twilio.com:3478?transport=tcp)")
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

	log.Println("Connecting to signaler with address", *raddr)

	u, err := url.Parse(*raddr)
	if err != nil {
		panic(err)
	}

	q := u.Query()
	q.Set("community", *community)
	q.Set("password", *password)
	u.RawQuery = q.Encode()

	adapter := wrtcconn.NewAdapter(
		u.String(),
		*key,
		strings.Split(*ice, ","),
		&wrtcconn.AdapterConfig{
			Timeout: *timeout,
			Verbose: *verbose,
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

	id := ""
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case id = <-ids:
				log.Println("Connected as", id)
			}
		}
	}()

	for {
		peer, conn, err := adapter.Accept()
		if err != nil {
			log.Println("Could not accept peer, continuing")

			continue
		}

		go func() {
			log.Println("Peer connected", peer)
			defer func() {
				log.Println("Peer disconnected", peer)
			}()

			for {
				buf := make([]byte, 1024)
				n, err := conn.Read(buf)
				if err != nil {
					if err := conn.Close(); err != nil {
						return
					}

					return
				}

				log.Println("Received message from peer", peer, "with length", n, string(buf))
			}
		}()

		go func() {
			defer func() {
				log.Println("Peer disconnected", peer)
			}()

			ticker := time.NewTicker(time.Second)
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					n, err := conn.Write([]byte("Hello from " + id + "!"))
					if err != nil {
						if err := conn.Close(); err != nil {
							return
						}

						return
					}

					log.Println("Sent message to peer", peer, "with length", n)
				}
			}
		}()
	}
}
