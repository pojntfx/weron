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

	fmt.Printf("\r.%v\n", *raddr)

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
			fmt.Printf("\r%v> ", id)
		case peer := <-adapter.Accept():
			fmt.Printf("\r+%v\n", peer.ID)
			fmt.Printf("\r%v> ", id)

			l := lines.Listener(0)

			go func() {
				defer func() {
					fmt.Printf("\r-%v\n", peer.ID)
					fmt.Printf("\r%v> ", id)

					l.Close()
				}()

				reader := bufio.NewScanner(peer.Conn)
				for reader.Scan() {
					fmt.Printf("\r%v: %v\n", peer.ID, reader.Text())
					fmt.Printf("\r%v> ", id)
				}
			}()

			go func() {
				for msg := range l.Ch() {
					if _, err := peer.Conn.Write(msg); err != nil {
						return
					}

					fmt.Printf("\r%v> ", id)
				}
			}()
		}
	}
}
