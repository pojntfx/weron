package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/pojntfx/webrtcfd/pkg/wrtcconn"
	"github.com/pojntfx/webrtcfd/pkg/wrtceth"
)

var (
	errMissingCommunity = errors.New("missing community")
	errMissingPassword  = errors.New("missing password")

	errMissingKey = errors.New("missing key")

	errInvalidTURNServerAddr  = errors.New("invalid TURN server address")
	errMissingTURNCredentials = errors.New("missing TURN server credentials")
)

func main() {
	raddr := flag.String("raddr", "wss://webrtcfd.herokuapp.com/", "Remote address")
	timeout := flag.Duration("timeout", time.Second*10, "Time to wait for connections")
	community := flag.String("community", "", "ID of community to join")
	password := flag.String("password", "", "Password for community")
	key := flag.String("key", "", "Encryption key for community")
	ice := flag.String("ice", "stun:stun.l.google.com:19302", "Comma-seperated list of STUN servers (in format stun:host:port) and TURN servers to use (in format username:credential@turn:host:port) (i.e. username:credential@turn:global.turn.twilio.com:3478?transport=tcp)")
	dev := flag.String("dev", "", "Name to give to the TAP device (i.e. weron0) (default is auto-generated; only supported on Linux, macOS and Windows)")
	mac := flag.String("mac", "", "MAC address to give to the TAP device (i.e. 3a:f8:de:7b:ef:52) (default is auto-generated; only supported on Linux)")
	parallel := flag.Int("parallel", runtime.NumCPU(), "Amount of threads to use to decode frames")
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

	adapter := wrtceth.NewAdapter(
		u.String(),
		*key,
		strings.Split(*ice, ","),
		&wrtceth.AdapterConfig{
			Device: *dev,
			OnSignalerConnect: func(s string) {
				log.Println("Connected to signaler as", s)
			},
			OnPeerConnect: func(s string) {
				log.Println("Connected to peer", s)
			},
			OnPeerDisconnected: func(s string) {
				log.Println("Disconnected from peer", s)
			},
			Parallel: *parallel,
			AdapterConfig: &wrtcconn.AdapterConfig{
				Timeout: *timeout,
				Verbose: *verbose,
				ID:      *mac,
			},
		},
		ctx,
	)

	log.Println("Connecting to signaler", *raddr)

	if err := adapter.Open(); err != nil {
		panic(err)
	}
	defer func() {
		if err := adapter.Close(); err != nil {
			panic(err)
		}
	}()

	if err := adapter.Wait(); err != nil {
		panic(err)
	}
}
