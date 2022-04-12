package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/pojntfx/webrtcfd/pkg/wrtcconn"
)

var (
	errMissingCommunity = errors.New("missing community")
	errMissingPassword  = errors.New("missing password")

	errMissingKey = errors.New("missing key")
)

const (
	typeGreeting = "greeting"
	typeKick     = "kick"
	typeWelcome  = "welcome"
)

type message struct {
	Type string `json:"type"`
}

type greeting struct {
	*message
	ID        string `json:"id"`
	Timestamp int64  `json:"timestamp"`
}

type welcome struct {
	*message
	ID string `json:"id"`
}

type kick struct {
	*message
}

func main() {
	raddr := flag.String("raddr", "wss://webrtcfd.herokuapp.com/", "Remote address")
	timeout := flag.Duration("timeout", time.Second*10, "Time to wait for connections")
	community := flag.String("community", "", "ID of community to join")
	password := flag.String("password", "", "Password for community")
	key := flag.String("key", "", "Encryption key for community")
	username := flag.String("username", "", "Username to send messages as (default is auto-generated)")
	channel := flag.String("channel", "wrtcchat", "Comma-seperated list of channel in community to join")
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

	fmt.Printf("\r\u001b[0K.%v\n", *raddr)

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
		strings.Split(*channel, ","),
		&wrtcconn.AdapterConfig{
			Timeout:    *timeout,
			Verbose:    *verbose,
			ForceRelay: *relay,
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

	ready := time.After(*kicks)
	oid := ""
	timestamp := time.Now().UnixNano()
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-ids:
			fmt.Printf("\r\u001b[0K%v.", id)
			ready = time.After(*kicks)
		case <-ready:
			oid = *username
			fmt.Printf("\r\u001b[0K%v!", oid)
		case peer := <-adapter.Accept():
			fmt.Printf("\r\u001b[0K+%v@%v\n", peer.PeerID, peer.ChannelID)
			fmt.Printf("\r\u001b[0K%v@%v> ", oid, peer.ChannelID)

			e := json.NewEncoder(peer.Conn)
			d := json.NewDecoder(peer.Conn)

			go func() {
				defer func() {
					fmt.Printf("\r\u001b[0K-%v@%v\n", peer.PeerID, peer.ChannelID)
					fmt.Printf("\r\u001b[0K%v@%v> ", oid, peer.ChannelID)
				}()

				for {
					var j interface{}
					if err := d.Decode(&j); err != nil {
						panic(err)
					}

					var msg message
					if err := mapstructure.Decode(j, &msg); err != nil {
						panic(err)
					}

					switch msg.Type {
					case typeGreeting:
						var gng greeting
						if err := mapstructure.Decode(j, &gng); err != nil {
							panic(err)
						}

						if gng.ID == *username {
							if oid == "" && gng.Timestamp < timestamp {
								panic("kicked")
							} else {
								if err := e.Encode(kick{
									message: &message{
										Type: typeKick,
									},
								}); err != nil {
									panic(err)
								}

								continue
							}
						}

						if err := e.Encode(welcome{
							message: &message{
								Type: typeWelcome,
							},
							ID: *username,
						}); err != nil {
							panic(err)
						}

						log.Println("Added peer", gng.ID)
					case typeWelcome:
						var wlc welcome
						if err := mapstructure.Decode(j, &wlc); err != nil {
							panic(err)
						}

						log.Println("Added peer", wlc.ID)
					case typeKick:
						panic("kicked")
					default:
						panic("unknown message type")
					}
				}
			}()

			if err := e.Encode(greeting{
				message: &message{
					Type: typeGreeting,
				},
				ID:        *username,
				Timestamp: timestamp,
			}); err != nil {
				panic(err)
			}
		}
	}
}
