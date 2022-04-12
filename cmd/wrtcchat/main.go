package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/pojntfx/webrtcfd/pkg/api/webrtc/v1/wrtcchat"
	"github.com/pojntfx/webrtcfd/pkg/wrtcconn"
	"github.com/teivah/broadcast"
)

var (
	errMissingCommunity = errors.New("missing community")
	errMissingPassword  = errors.New("missing password")

	errMissingKey       = errors.New("missing key")
	errMissingUsername  = errors.New("missing username")
	errUsernameRejected = errors.New("username has been rejected")
)

func main() {
	raddr := flag.String("raddr", "wss://webrtcfd.herokuapp.com/", "Remote address")
	timeout := flag.Duration("timeout", time.Second*10, "Time to wait for connections")
	community := flag.String("community", "", "ID of community to join")
	password := flag.String("password", "", "Password for community")
	key := flag.String("key", "", "Encryption key for community")
	username := flag.String("username", "", "Username to send messages as (default is auto-generated)")
	channel := flag.String("channel", "wrtcchat.primary", "Comma-seperated list of channels in community to join")
	control := flag.String("control", "wrtcchat.control", "Control channel in community to join")
	rejection := flag.Duration("rejection", time.Second*5, "Time to wait for rejections")
	ice := flag.String("ice", "stun:stun.l.google.com:19302", "Comma-seperated list of STUN servers (in format stun:host:port) and TURN servers to use (in format username:credential@turn:host:port) (i.e. username:credential@turn:global.turn.twilio.com:3478?transport=tcp)")
	relay := flag.Bool("force-relay", false, "Force usage of TURN servers")
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

	if strings.TrimSpace(*username) == "" {
		panic(errMissingUsername)
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
		append([]string{*control}, strings.Split(*channel, ",")...),
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

	lines := broadcast.NewRelay[[]byte]()
	go func() {
		reader := bufio.NewScanner(os.Stdin)

		for reader.Scan() {
			lines.NotifyCtx(ctx, []byte(reader.Text()+"\n"))
		}
	}()

	oid := ""

	ready := broadcast.NewRelay[struct{}]()
	rejections := make(chan struct{})
	errs := make(chan error)

	peers := map[string]*wrtcconn.Peer{}
	var peerLock sync.Mutex

	readies := map[string]*broadcast.Relay[string]{}
	var readyLock sync.Mutex

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errs:
			panic(err)
		case id := <-ids:
			go func() {
				fmt.Printf("\r\u001b[0K%v.", id)

				time.Sleep(*rejection / 2)

				d, err := json.Marshal(wrtcchat.NewApplication(*username))
				if err != nil {
					errs <- err

					return
				}

				peerLock.Lock()
				for _, peer := range peers {
					if _, err := peer.Conn.Write(d); err != nil {
						if *verbose {
							log.Println("Could not write to peer, skipping")
						}

						continue
					}
				}
				peerLock.Unlock()

				for {
					select {
					case <-ctx.Done():
						return
					case <-rejections:
						errs <- errUsernameRejected

						return
					case <-time.After(*rejection):
						oid = *username
						ready.NotifyCtx(ctx, struct{}{})

						log.Println("Claimed successfully!")

						d, err := json.Marshal(wrtcchat.NewAcceptance(*username))
						if err != nil {
							errs <- err

							return
						}

						peerLock.Lock()
						for _, peer := range peers {
							if _, err := peer.Conn.Write(d); err != nil {
								if *verbose {
									log.Println("Could not write to peer, skipping")
								}

								continue
							}
						}
						peerLock.Unlock()

						return
					}
				}
			}()
		case peer := <-adapter.Accept():
			readyLock.Lock()
			if _, ok := readies[peer.PeerID]; !ok {
				readies[peer.PeerID] = broadcast.NewRelay[string]()
			}
			readyLock.Unlock()

			switch peer.ChannelID {
			case *control:
				go func() {
					peerLock.Lock()
					peers[peer.PeerID] = peer
					peerLock.Unlock()

					defer func() {
						peerLock.Lock()
						delete(peers, peer.PeerID)
						peerLock.Unlock()

						readyLock.Lock()
						delete(readies, peer.PeerID)
						readyLock.Unlock()

						// Handle JSON parser errors when reading from connection
						if err := recover(); err != nil {
							if *verbose {
								log.Println("Could not read from peer, stopping")

								return
							}
						}
					}()

					d := json.NewDecoder(peer.Conn)

					for {
						var j interface{}
						if err := d.Decode(&j); err != nil {
							if err == io.EOF {
								if *verbose {
									log.Println("Could not read from peer, stopping")
								}

								return
							}

							if *verbose {
								log.Println("Could not read from peer, skipping")
							}

							continue
						}

						var message wrtcchat.Message
						if err := mapstructure.Decode(j, &message); err != nil {
							if *verbose {
								log.Println("Could not parse message from peer, skipping")
							}

							continue
						}

						switch message.Type {
						case wrtcchat.TypeApplication:
							var application wrtcchat.Application
							if err := mapstructure.Decode(j, &application); err != nil {
								if *verbose {
									log.Println("Could not parse message from peer, skipping")
								}

								continue
							}

							if oid == "" {
								r := ready.Listener(0)

								<-r.Ch()

								r.Close()
							}

							if _, ok := peers[application.ID]; ok || application.ID == oid {
								d, err := json.Marshal(wrtcchat.NewRejection())
								if err != nil {
									if *verbose {
										log.Println("Could not marshal rejection, skipping")
									}

									continue
								}

								if _, err := peer.Conn.Write(d); err != nil {
									if *verbose {
										log.Println("Could not write to peer, skipping")
									}

									continue
								}

								continue
							}

							readyLock.Lock()
							readies[peer.PeerID].NotifyCtx(ctx, application.ID)
							readyLock.Unlock()
						case wrtcchat.TypeAcceptance:
							var acceptance wrtcchat.Acceptance
							if err := mapstructure.Decode(j, &acceptance); err != nil {
								if *verbose {
									log.Println("Could not parse message from peer, skipping")
								}

								continue
							}

							readyLock.Lock()
							readies[peer.PeerID].NotifyCtx(ctx, acceptance.ID)
							readyLock.Unlock()
						case wrtcchat.TypeRejection:
							rejections <- struct{}{}

							return
						default:
							if *verbose {
								log.Println("Got unknown message type, skipping")
							}

							continue
						}
					}
				}()
			default:
				go func() {
					if oid == "" {
						r := ready.Listener(0)

						<-r.Ch()

						r.Close()
					}

					r := readies[peer.PeerID].Listener(0)

					pid := ""
					select {
					case <-ctx.Done():
						return
					case pid = <-r.Ch():
						r.Close()
					}

					fmt.Printf("\r\u001b[0K+%v@%v\n", pid, peer.ChannelID)
					fmt.Printf("\r\u001b[0K%v@%v> ", oid, peer.ChannelID)

					l := lines.Listener(0)

					go func() {
						defer func() {
							fmt.Printf("\r\u001b[0K-%v@%v\n", pid, peer.ChannelID)
							fmt.Printf("\r\u001b[0K%v@%v> ", oid, peer.ChannelID)

							l.Close()
						}()

						reader := bufio.NewScanner(peer.Conn)
						for reader.Scan() {
							fmt.Printf("\r\u001b[0K%v@%v: %v\n", pid, peer.ChannelID, reader.Text())
							fmt.Printf("\r\u001b[0K%v@%v> ", oid, peer.ChannelID)
						}
					}()

					go func() {
						for msg := range l.Ch() {
							if _, err := peer.Conn.Write(msg); err != nil {
								if *verbose {
									log.Println("Could not write to peer, stopping")
								}

								return
							}

							fmt.Printf("\r\u001b[0K%v@%v> ", oid, peer.ChannelID)
						}
					}()
				}()
			}
		}
	}
}
