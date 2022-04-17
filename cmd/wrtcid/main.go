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
	"sync"
	"time"

	"github.com/mitchellh/mapstructure"
	v1 "github.com/pojntfx/webrtcfd/pkg/api/webrtc/v1"
	"github.com/pojntfx/webrtcfd/pkg/wrtcconn"
)

var (
	errMissingCommunity = errors.New("missing community")
	errMissingPassword  = errors.New("missing password")

	errMissingKey       = errors.New("missing key")
	errMissingUsernames = errors.New("missing usernames")

	errAllUsernamesClaimed = errors.New("all specified usernames are already claimed")
)

func main() {
	raddr := flag.String("raddr", "wss://webrtcfd.herokuapp.com/", "Remote address")
	timeout := flag.Duration("timeout", time.Second*10, "Time to wait for connections")
	community := flag.String("community", "", "ID of community to join")
	password := flag.String("password", "", "Password for community")
	key := flag.String("key", "", "Encryption key for community")
	usernames := flag.String("usernames", "", "Comma-seperated list of username to try and claim")
	channel := flag.String("channel", "wrtcid.primary", "Comma-seperated list of channels in community to join")
	idChannel := flag.String("id-channel", "wrtcid.id", "Channel for ID negotiation in community to join")
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

	if strings.TrimSpace(*usernames) == "" {
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

	adapter := wrtcconn.NewAdapter(
		u.String(),
		*key,
		strings.Split(*ice, ","),
		append([]string{*idChannel}, strings.Split(*channel, ",")...),
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

	var candidatesLock sync.Mutex
	candidates := map[string]struct{}{}
	id := ""
	timestamp := time.Now().UnixNano()

	peers := map[string]map[string]*wrtcconn.Peer{}
	var peersLock sync.Mutex

	namedPeers := make(chan *wrtcconn.Peer)
	var namedPeersLock sync.Mutex
	namedPeersCond := sync.NewCond(&namedPeersLock)

	ready := time.NewTimer(*timeout + *kicks)
	errs := make(chan error)
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errs:
			panic(err)
		case sid := <-ids:
			candidatesLock.Lock()
			candidates = map[string]struct{}{}
			for _, username := range strings.Split(*usernames, ",") {
				candidates[username] = struct{}{}
			}
			id = ""
			candidatesLock.Unlock()

			fmt.Printf("%v.\n", sid)

			ready.Stop()
			ready.Reset(*kicks)

		case <-ready.C:
			candidatesLock.Lock()
			for username := range candidates {
				id = username

				break
			}
			candidates = map[string]struct{}{}
			candidatesLock.Unlock()

			if id == "" {
				panic(errAllUsernamesClaimed)
			}

			fmt.Printf("%v!\n", id)
			namedPeersCond.Broadcast()

			peersLock.Lock()
			for _, peer := range peers {
				if *verbose {
					log.Println("Sending claimed")
				}

				d, err := json.Marshal(v1.NewClaimed(id))
				if err != nil {
					if *verbose {
						log.Println("Could not marshal claimed")
					}

					continue
				}

				if _, err := peer[*idChannel].Conn.Write(d); err != nil {
					if *verbose {
						log.Println("Could not send to peer, skipping")
					}

					continue
				}
			}
			peersLock.Unlock()
		case peer := <-namedPeers:
			go func() {
				if id == "" {
					namedPeersCond.L.Lock()
					namedPeersCond.Wait()
					namedPeersCond.L.Unlock()
				}

				log.Println("Connected to peer with ID", peer.PeerID, "on channel", peer.ChannelID)
			}()
		case peer := <-adapter.Accept():
			rid := peer.PeerID

			peersLock.Lock()
			for candidate, p := range peers {
				for _, c := range p {
					if c.PeerID == peer.PeerID {
						rid = candidate

						break
					}
				}
			}
			if _, ok := peers[rid]; !ok {
				peers[rid] = map[string]*wrtcconn.Peer{}
			}
			peers[rid][peer.ChannelID] = peer
			if rid != peer.PeerID && peer.ChannelID != *idChannel {
				namedPeers <- &wrtcconn.Peer{
					PeerID:    rid,
					ChannelID: peer.ChannelID,
					Conn:      peer.Conn,
				}
			}
			peersLock.Unlock()

			if peer.ChannelID == *idChannel {
				go func() {
					e := json.NewEncoder(peer.Conn)
					d := json.NewDecoder(peer.Conn)

					defer func() {
						if err := recover(); err != nil {
							if *verbose {
								log.Println("Could not read/write from peer, stopping")

								return
							}
						}

						if rid != peer.PeerID {
							fmt.Printf("-%v@%v\n", rid, peer.ChannelID)
						}

						peersLock.Lock()
						if _, ok := peers[rid]; !ok {
							delete(peers[rid], peer.ChannelID)

							if len(peers[rid]) <= 0 {
								delete(peers, rid)
							}
						}
						peersLock.Unlock()
					}()

					greet := func() {
						if *verbose {
							log.Println("Sending greeting")
						}

						if id == "" {
							if err := e.Encode(v1.NewGreeting(candidates, timestamp)); err != nil {
								if *verbose {
									log.Println("Could not send to peer, stopping")
								}

								return
							}
						} else {
							if err := e.Encode(v1.NewGreeting(map[string]struct{}{id: {}}, timestamp)); err != nil {
								if *verbose {
									log.Println("Could not send to peer, stopping")
								}

								return
							}

							if *verbose {
								log.Println("Sending claimed")
							}

							if err := e.Encode(v1.NewClaimed(id)); err != nil {
								if *verbose {
									log.Println("Could not send to peer, stopping")
								}

								return
							}
						}
					}

					greet()

				l:
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

							if *verbose {
								log.Println("Received greeting from", gng.IDs)
							}

							for gngID := range gng.IDs {
								if _, ok := candidates[gngID]; id == "" && ok && timestamp < gng.Timestamp {
									if *verbose {
										log.Println("Sending backoff to", gngID)
									}

									if err := e.Encode(v1.NewBackoff()); err != nil {
										if *verbose {
											log.Println("Could not send to peer, stopping")
										}

										return
									}

									continue l
								}
							}

							if _, ok := gng.IDs[id]; ok {
								if *verbose {
									log.Println("Sending kick to", id)
								}

								if err := e.Encode(v1.NewKick(id)); err != nil {
									if *verbose {
										log.Println("Could not send to peer, stopping")
									}

									return
								}
							}
						case v1.TypeKick:
							var kck v1.Kick
							if err := mapstructure.Decode(j, &kck); err != nil {
								if *verbose {
									log.Println("Could not decode from peer, skipping")
								}

								continue
							}

							if *verbose {
								log.Println("Received kick from", kck.ID)
							}

							candidatesLock.Lock()
							delete(candidates, kck.ID)
							candidatesLock.Unlock()
						case v1.TypeBackoff:
							if *verbose {
								log.Println("Received backoff")
							}

							ready.Stop()

							time.Sleep(*kicks)

							greet()

							ready.Reset(*kicks)
						case v1.TypeClaimed:
							var clm v1.Claimed
							if err := mapstructure.Decode(j, &clm); err != nil {
								if *verbose {
									log.Println("Could not decode from peer, skipping")
								}

								continue
							}

							if *verbose {
								log.Println("Received claimed from", clm.ID)
							}

							rid = clm.ID

							if _, ok := peers[rid]; !ok {
								fmt.Printf("+%v@%v\n", rid, peer.ChannelID)
							}

							peersLock.Lock()
							if _, ok := peers[rid]; !ok {
								peers[rid] = map[string]*wrtcconn.Peer{}
							}
							for key, value := range peers[peer.PeerID] {
								peers[rid][key] = value

								if value.ChannelID != *idChannel {
									namedPeers <- &wrtcconn.Peer{
										PeerID:    rid,
										ChannelID: value.ChannelID,
										Conn:      value.Conn,
									}
								}
							}
							delete(peers, peer.PeerID)
							peersLock.Unlock()
						default:
							if *verbose {
								log.Println("Could not handle unknown message type from peer, skipping")
							}

							continue
						}
					}
				}()
			}
		}
	}
}
