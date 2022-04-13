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
	v1 "github.com/pojntfx/webrtcfd/pkg/api/webrtc/v1"
	"github.com/pojntfx/webrtcfd/pkg/wrtcconn"
)

var (
	errMissingCommunity = errors.New("missing community")
	errMissingPassword  = errors.New("missing password")

	errMissingKey = errors.New("missing key")
	errKicked     = errors.New("kicked")
)

func main() {
	raddr := flag.String("raddr", "wss://webrtcfd.herokuapp.com/", "Remote address")
	timeout := flag.Duration("timeout", time.Second*10, "Time to wait for connections")
	community := flag.String("community", "", "ID of community to join")
	password := flag.String("password", "", "Password for community")
	key := flag.String("key", "", "Encryption key for community")
	username := flag.String("username", "", "Username to send messages as (default is auto-generated)")
	channel := flag.String("channel", "wrtcid", "Comma-seperated list of channel in community to join")
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

	fmt.Printf(".%v\n", *raddr)

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
	errs := make(chan error)
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errs:
			panic(err)
		case id := <-ids:
			fmt.Printf("%v.\n", id)
			ready = time.After(*kicks)
		case <-ready:
			oid = *username
			fmt.Printf("%v!\n", oid)
		case peer := <-adapter.Accept():
			e := json.NewEncoder(peer.Conn)
			d := json.NewDecoder(peer.Conn)

			go func() {
				rid := ""
				defer func() {
					if rid != "" {
						fmt.Printf("-%v@%v\n", rid, peer.ChannelID)
					}

					// Handle JSON parser errors when reading/writing from connection
					if err := recover(); err != nil {
						if *verbose {
							log.Println("Could not read/write from peer, stopping")

							return
						}
					}
				}()

				for {
					var j interface{}
					if err := d.Decode(&j); err != nil {
						if *verbose {
							log.Println("Could not read from peer, stopping")
						}

						return
					}

					var msg v1.Message
					if err := mapstructure.Decode(j, &msg); err != nil {
						if *verbose {
							log.Println("Could not decode from peer, skipping")
						}

						continue
					}

					switch msg.Type {
					case v1.TypeGreeting:
						var gng v1.Greeting
						if err := mapstructure.Decode(j, &gng); err != nil {
							if *verbose {
								log.Println("Could not decode from peer, skipping")
							}

							continue
						}

						if gng.ID == *username {
							if oid == "" && gng.Timestamp < timestamp {
								errs <- errKicked

								return
							} else {
								if err := e.Encode(v1.NewKick()); err != nil {
									if *verbose {
										log.Println("Could not send to peer, stopping")
									}

									return
								}

								continue
							}
						}

						if err := e.Encode(v1.NewWelcome(*username)); err != nil {
							if *verbose {
								log.Println("Could not send to peer, stopping")
							}

							return
						}

						rid = gng.ID

						fmt.Printf("+%v@%v\n", rid, peer.ChannelID)
					case v1.TypeWelcome:
						var wlc v1.Welcome
						if err := mapstructure.Decode(j, &wlc); err != nil {
							if *verbose {
								log.Println("Could not decode from peer, skipping")
							}

							continue
						}

						rid = wlc.ID

						fmt.Printf("+%v@%v\n", rid, peer.ChannelID)
					case v1.TypeKick:
						errs <- errKicked

						return
					default:
						if *verbose {
							log.Println("Could not handle unknown message type from peer, skipping")
						}

						continue
					}
				}
			}()

			if err := e.Encode(v1.NewGreeting(*username, timestamp)); err != nil {
				if *verbose {
					log.Println("Could not send to peer, stopping")
				}

				return
			}
		}
	}
}
