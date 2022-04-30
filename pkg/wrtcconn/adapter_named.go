package wrtcconn

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"

	"github.com/mitchellh/mapstructure"
	v1 "github.com/pojntfx/weron/pkg/api/webrtc/v1"
	"github.com/pojntfx/weron/pkg/services"
)

var (
	ErrAllNamesClaimed = errors.New("all available names have been claimed")

	json = jsoniter.ConfigCompatibleWithStandardLibrary
)

type NamedAdapterConfig struct {
	*AdapterConfig
	IDChannel   string
	Names       []string
	Kicks       time.Duration
	IsIDClaimed func(theirs map[string]struct{}, ours string) bool
}

type NamedAdapter struct {
	signaler string
	key      string
	ice      []string
	channels []string
	config   *NamedAdapterConfig
	ctx      context.Context

	cancel        context.CancelFunc
	adapter       *Adapter
	ids           chan string
	names         chan string
	errs          chan error
	acceptedPeers chan *Peer
}

func NewNamedAdapter(
	signaler string,
	key string,
	ice []string,
	channels []string,
	config *NamedAdapterConfig,
	ctx context.Context,
) *NamedAdapter {
	ictx, cancel := context.WithCancel(ctx)

	if config == nil {
		config = &NamedAdapterConfig{}
	}

	if config.IDChannel == "" {
		config.IDChannel = services.IDGeneral
	}

	if config.IsIDClaimed == nil {
		config.IsIDClaimed = func(ids map[string]struct{}, id string) bool {
			_, ok := ids[id]

			return ok
		}
	}

	return &NamedAdapter{
		signaler: signaler,
		key:      key,
		ice:      ice,
		channels: channels,
		config:   config,
		ctx:      ictx,

		cancel:        cancel,
		ids:           make(chan string),
		names:         make(chan string),
		errs:          make(chan error),
		acceptedPeers: make(chan *Peer),
	}
}

func (a *NamedAdapter) Open() (chan string, error) {
	ready := time.NewTimer(a.config.Timeout + a.config.Kicks)

	a.config.AdapterConfig.OnSignalerReconnect = func() {
		ready.Stop()
		ready.Reset(a.config.Timeout + a.config.Kicks)
	}

	a.adapter = NewAdapter(
		a.signaler,
		a.key,
		strings.Split(strings.Join(a.ice, ","), ","),
		append([]string{a.config.IDChannel}, a.channels...),
		a.config.AdapterConfig,
		a.ctx,
	)

	var err error
	a.ids, err = a.adapter.Open()
	if err != nil {
		return nil, err
	}

	var candidatesLock sync.Mutex
	candidates := map[string]struct{}{}
	id := ""
	timestamp := time.Now().UnixNano()

	peers := map[string]map[string]*Peer{}
	var peersLock sync.Mutex

	namedPeers := make(chan *Peer)
	var namedPeersLock sync.Mutex
	namedPeersCond := sync.NewCond(&namedPeersLock)

	go func() {
		for {
			select {
			case <-a.ctx.Done():
				return
			case sid := <-a.ids:
				candidatesLock.Lock()
				candidates = map[string]struct{}{}
				for _, username := range a.config.Names {
					candidates[username] = struct{}{}
				}
				id = ""
				candidatesLock.Unlock()

				if a.config.Verbose {
					log.Println("Got ID", sid)
				}

				ready.Stop()
				ready.Reset(a.config.Kicks)
			case <-ready.C:
				candidatesLock.Lock()
				for username := range candidates {
					id = username

					break
				}
				candidates = map[string]struct{}{}
				candidatesLock.Unlock()

				if id == "" {
					a.errs <- ErrAllNamesClaimed

					return
				}

				a.names <- id
				namedPeersCond.Broadcast()

				peersLock.Lock()
				for _, peer := range peers {
					if a.config.Verbose {
						log.Println("Sending claimed")
					}

					d, err := json.Marshal(v1.NewClaimed(id))
					if err != nil {
						if a.config.Verbose {
							log.Println("Could not marshal claimed")
						}

						continue
					}

					if _, err := peer[a.config.IDChannel].Conn.Write(d); err != nil {
						if a.config.Verbose {
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

					a.acceptedPeers <- peer
				}()
			case peer := <-a.adapter.Accept():
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
					peers[rid] = map[string]*Peer{}
				}
				peers[rid][peer.ChannelID] = peer
				if rid != peer.PeerID && peer.ChannelID != a.config.IDChannel {
					namedPeers <- &Peer{
						PeerID:    rid,
						ChannelID: peer.ChannelID,
						Conn:      peer.Conn,
					}
				}
				peersLock.Unlock()

				if peer.ChannelID == a.config.IDChannel {
					go func() {
						e := json.NewEncoder(peer.Conn)
						d := json.NewDecoder(peer.Conn)

						defer func() {
							if err := recover(); err != nil {
								if a.config.Verbose {
									log.Println("Could not read/write from peer, stopping")

									return
								}
							}

							if rid != peer.PeerID {
								if a.config.Verbose {
									log.Println("Disconnected from peer", rid, "and channel", peer.ChannelID)
								}
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
							if a.config.Verbose {
								log.Println("Sending greeting")
							}

							if id == "" {
								if err := e.Encode(v1.NewGreeting(candidates, timestamp)); err != nil {
									if a.config.Verbose {
										log.Println("Could not send to peer, stopping")
									}

									return
								}
							} else {
								if err := e.Encode(v1.NewGreeting(map[string]struct{}{id: {}}, timestamp)); err != nil {
									if a.config.Verbose {
										log.Println("Could not send to peer, stopping")
									}

									return
								}

								if a.config.Verbose {
									log.Println("Sending claimed")
								}

								if err := e.Encode(v1.NewClaimed(id)); err != nil {
									if a.config.Verbose {
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
								if a.config.Verbose {
									log.Println("Could not read from peer, stopping")
								}

								return
							}

							var msg v1.Message
							if err := mapstructure.Decode(j, &msg); err != nil {
								if a.config.Verbose {
									log.Println("Could not decode from peer, skipping")
								}

								continue
							}

							switch msg.Type {
							case v1.TypeGreeting:
								var gng v1.Greeting
								if err := mapstructure.Decode(j, &gng); err != nil {
									if a.config.Verbose {
										log.Println("Could not decode from peer, skipping")
									}

									continue
								}

								if a.config.Verbose {
									log.Println("Received greeting from", gng.IDs)
								}

								for gngID := range gng.IDs {
									if _, ok := candidates[gngID]; id == "" && ok && timestamp < gng.Timestamp {
										if a.config.Verbose {
											log.Println("Sending backoff to", gngID)
										}

										if err := e.Encode(v1.NewBackoff()); err != nil {
											if a.config.Verbose {
												log.Println("Could not send to peer, stopping")
											}

											return
										}

										continue l
									}
								}

								if a.config.IsIDClaimed(gng.IDs, id) {
									if a.config.Verbose {
										log.Println("Sending kick to", id)
									}

									if err := e.Encode(v1.NewKick(id)); err != nil {
										if a.config.Verbose {
											log.Println("Could not send to peer, stopping")
										}

										return
									}
								}
							case v1.TypeKick:
								var kck v1.Kick
								if err := mapstructure.Decode(j, &kck); err != nil {
									if a.config.Verbose {
										log.Println("Could not decode from peer, skipping")
									}

									continue
								}

								if a.config.Verbose {
									log.Println("Received kick from", kck.ID)
								}

								candidatesLock.Lock()
								delete(candidates, kck.ID)
								candidatesLock.Unlock()
							case v1.TypeBackoff:
								if a.config.Verbose {
									log.Println("Received backoff")
								}

								ready.Stop()

								time.Sleep(a.config.Kicks)

								greet()

								ready.Reset(a.config.Kicks)
							case v1.TypeClaimed:
								var clm v1.Claimed
								if err := mapstructure.Decode(j, &clm); err != nil {
									if a.config.Verbose {
										log.Println("Could not decode from peer, skipping")
									}

									continue
								}

								if a.config.Verbose {
									log.Println("Received claimed from", clm.ID)
								}

								rid = clm.ID

								if _, ok := peers[rid]; !ok {
									if a.config.Verbose {
										log.Println("Connected to peer", rid, "and channel", peer.ChannelID)
									}
								}

								peersLock.Lock()
								if _, ok := peers[rid]; !ok {
									peers[rid] = map[string]*Peer{}
								}
								for key, value := range peers[peer.PeerID] {
									peers[rid][key] = value

									if value.ChannelID != a.config.IDChannel {
										namedPeers <- &Peer{
											PeerID:    rid,
											ChannelID: value.ChannelID,
											Conn:      value.Conn,
										}
									}
								}
								delete(peers, peer.PeerID)
								peersLock.Unlock()
							default:
								if a.config.Verbose {
									log.Println("Could not handle unknown message type from peer, skipping")
								}

								continue
							}
						}
					}()
				}
			}
		}
	}()

	return a.names, nil
}

func (a *NamedAdapter) Close() error {
	return a.adapter.Close()
}

func (a *NamedAdapter) Err() chan error {
	return a.errs
}

func (a *NamedAdapter) Accept() chan *Peer {
	return a.acceptedPeers
}
