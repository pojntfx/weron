package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/pojntfx/webrtcfd/pkg/wrtcconn"
	"github.com/pojntfx/webrtcfd/pkg/wrtcip"
)

var (
	errMissingCommunity = errors.New("missing community")
	errMissingPassword  = errors.New("missing password")
	errMissingKey       = errors.New("missing key")

	errMissingIPs  = errors.New("no IP(s) provided")
	errInvalidCIDR = errors.New("invalid CIDR notation for IPs")
)

func main() {
	raddr := flag.String("raddr", "wss://webrtcfd.herokuapp.com/", "Remote address")
	timeout := flag.Duration("timeout", time.Second*10, "Time to wait for connections")
	community := flag.String("community", "", "ID of community to join")
	password := flag.String("password", "", "Password for community")
	key := flag.String("key", "", "Encryption key for community")
	ice := flag.String("ice", "stun:stun.l.google.com:19302", "Comma-seperated list of STUN servers (in format stun:host:port) and TURN servers to use (in format username:credential@turn:host:port) (i.e. username:credential@turn:global.turn.twilio.com:3478?transport=tcp)")
	dev := flag.String("dev", "", "Name to give to the TUN device (i.e. weron0) (default is auto-generated; only supported on Linux and macOS)")
	ips := flag.String("ips", "", "Comma-seperated list of IP addresses to give to the TUN device (i.e. 2001:db8::1/32,192.0.2.1/24) (on Windows, only one IPv4 and one IPv6 address are supported; macOS only supports IPv6)")
	parallel := flag.Int("parallel", runtime.NumCPU(), "Amount of threads to use to decode packets")
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

	if strings.TrimSpace(*ips) == "" {
		panic(errMissingIPs)
	}

	for _, ip := range strings.Split(*ips, ",") {
		if _, _, err := net.ParseCIDR(ip); err != nil {
			panic(errInvalidCIDR)
		}
	}

	u, err := url.Parse(*raddr)
	if err != nil {
		panic(err)
	}

	q := u.Query()
	q.Set("community", *community)
	q.Set("password", *password)
	u.RawQuery = q.Encode()

	adapter := wrtcip.NewAdapter(
		u.String(),
		*key,
		strings.Split(*ice, ","),
		&wrtcip.AdapterConfig{
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
			IPs:      strings.Split(*ips, ","),
			Parallel: *parallel,
			AdapterConfig: &wrtcconn.AdapterConfig{
				Timeout: *timeout,
				Verbose: *verbose,
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
