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
	"github.com/songgao/water"
	"github.com/teivah/broadcast"
	"github.com/vishvananda/netlink"
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
	dev := flag.String("dev", "", "Name to give to the TAP device (i.e. weron0)")
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

	log.Println("Connecting to signaler", *raddr)

	ids, err := adapter.Open()
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := adapter.Close(); err != nil {
			panic(err)
		}
	}()

	tap, err := water.New(water.Config{
		DeviceType: water.TAP,
		PlatformSpecificParams: water.PlatformSpecificParams{
			Name: *dev,
		},
	})
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := tap.Close(); err != nil {
			panic(err)
		}
	}()

	link, err := netlink.LinkByName(tap.Name())
	if err != nil {
		panic(err)
	}

	if err := netlink.LinkSetHardwareAddr(link, link.Attrs().HardwareAddr); err != nil {
		panic(err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		panic(err)
	}

	lines := broadcast.NewRelay[[]byte]()
	go func() {
		for {
			buf := make([]byte, link.Attrs().MTU)

			if _, err := tap.Read(buf); err != nil {
				return
			}

			lines.Broadcast([]byte(buf))
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case id := <-ids:
			log.Println("Connected to signaler as", id)
		case peer := <-adapter.Accept():
			log.Println("Connected to peer", peer.ID)

			l := lines.Listener(0)

			go func() {
				defer func() {
					log.Println("Disconnected from peer", peer.ID)
				}()

				for {
					buf := make([]byte, link.Attrs().MTU)

					if _, err := peer.Conn.Read(buf); err != nil {
						return
					}

					if _, err := tap.Write(buf); err != nil {
						return
					}
				}
			}()

			go func() {
				for msg := range l.Ch() {
					if _, err := peer.Conn.Write(msg); err != nil {
						return
					}
				}
			}()
		}
	}
}
