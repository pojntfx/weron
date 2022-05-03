package wrtcconn

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/rs/zerolog/log"

	"github.com/mitchellh/mapstructure"
	v1 "github.com/pojntfx/weron/pkg/api/webrtc/v1"
	"github.com/pojntfx/weron/pkg/services"
)

var (
	ErrAllNamesClaimed = errors.New("all available names have been claimed") // All specified usernames have already been claimed by other peers

	json = jsoniter.ConfigCompatibleWithStandardLibrary
)

// NamedAdapterConfig configures the adapter
type NamedAdapterConfig struct {
	*AdapterConfig
	IDChannel   string                                             // Channel to use for ID negotiation
	Names       []string                                           // Names to try and claim one of
	Kicks       time.Duration                                      // Time to wait for kicks before claiming names
	IsIDClaimed func(theirs map[string]struct{}, ours string) bool // Handler to be called when asked to compare own ID with an incoming greeting
}

// NamedAdapter provides a connection service with name conflict prevention
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

// NewNamedAdapter creates the adapter
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

// Open connects the adapter to the signaler
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

				log.Debug().Str("id", sid).Msg("Claimed ID")

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
					log.Debug().Str("id", id).Msg("Sending claimed")

					d, err := json.Marshal(v1.NewClaimed(id))
					if err != nil {
						log.Debug().
							Str("id", id).
							Err(err).
							Msg("Could not marshal claimed")

						continue
					}

					if _, err := peer[a.config.IDChannel].Conn.Write(d); err != nil {
						log.Debug().
							Str("channelID", peer[a.config.IDChannel].ChannelID).
							Str("peerID", peer[a.config.IDChannel].PeerID).
							Msg("Could not write to peer, stopping")

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
								log.Debug().
									Err(err.(error)).
									Str("channelID", peer.ChannelID).
									Str("peerID", rid).
									Msg("Could not read/write from peer, stopping")
							}

							if rid != peer.PeerID {
								log.Debug().
									Str("channelID", peer.ChannelID).
									Str("peerID", rid).
									Msg("Disconnected from peer")
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
							log.Debug().
								Str("channelID", peer.ChannelID).
								Str("peerID", rid).
								Int("candidates", len(candidates)).
								Int64("timestamp", timestamp).
								Msg("Sending greeting")

							if id == "" {
								if err := e.Encode(v1.NewGreeting(candidates, timestamp)); err != nil {
									log.Debug().
										Err(err).
										Str("channelID", peer.ChannelID).
										Str("peerID", rid).
										Msg("Could not write greeting to peer, stopping")

									return
								}
							} else {
								if err := e.Encode(v1.NewGreeting(map[string]struct{}{id: {}}, timestamp)); err != nil {
									log.Debug().
										Err(err).
										Str("channelID", peer.ChannelID).
										Str("peerID", rid).
										Msg("Could not write greeting to peer, stopping")

									return
								}

								log.Debug().
									Str("channelID", peer.ChannelID).
									Str("peerID", rid).
									Str("id", id).
									Msg("Sending claimed")

								if err := e.Encode(v1.NewClaimed(id)); err != nil {
									log.Debug().
										Err(err).
										Str("channelID", peer.ChannelID).
										Str("peerID", rid).
										Msg("Could not write claimed to peer, stopping")

									return
								}
							}
						}

						greet()

					l:
						for {
							var j interface{}
							if err := d.Decode(&j); err != nil {
								log.Debug().
									Err(err).
									Str("channelID", peer.ChannelID).
									Str("peerID", rid).
									Msg("Could not read from peer, stopping")

								return
							}

							var message v1.Message
							if err := mapstructure.Decode(j, &message); err != nil {
								log.Debug().
									Err(err).
									Str("channelID", peer.ChannelID).
									Str("peerID", rid).
									Msg("Could not decode message from peer, stopping")

								continue
							}

							switch message.Type {
							case v1.TypeGreeting:
								var gng v1.Greeting
								if err := mapstructure.Decode(j, &gng); err != nil {
									log.Debug().
										Err(err).
										Str("channelID", peer.ChannelID).
										Str("peerID", rid).
										Msg("Could not decode greeting from peer, stopping")

									continue
								}

								log.Debug().
									Err(err).
									Str("channelID", peer.ChannelID).
									Str("peerID", rid).
									Msg("Received greeting")

								for gngID := range gng.IDs {
									if _, ok := candidates[gngID]; id == "" && ok && timestamp < gng.Timestamp {
										log.Debug().
											Str("channelID", peer.ChannelID).
											Str("peerID", rid).
											Str("id", gngID).
											Msg("Sending backoff")

										if err := e.Encode(v1.NewBackoff()); err != nil {
											log.Debug().
												Err(err).
												Str("channelID", peer.ChannelID).
												Str("peerID", rid).
												Msg("Could not write backoff to peer, stopping")

											return
										}

										continue l
									}
								}

								if a.config.IsIDClaimed(gng.IDs, id) {
									log.Debug().
										Str("channelID", peer.ChannelID).
										Str("peerID", rid).
										Str("id", id).
										Msg("Sending kick")

									if err := e.Encode(v1.NewKick(id)); err != nil {
										log.Debug().
											Err(err).
											Str("channelID", peer.ChannelID).
											Str("peerID", rid).
											Str("id", id).
											Msg("Could not send backoff to peer, stopping")

										return
									}
								}
							case v1.TypeKick:
								var kck v1.Kick
								if err := mapstructure.Decode(j, &kck); err != nil {
									log.Debug().
										Err(err).
										Str("channelID", peer.ChannelID).
										Str("peerID", rid).
										Msg("Could not decode kick from peer, stopping")

									continue
								}

								log.Debug().
									Err(err).
									Str("channelID", peer.ChannelID).
									Str("peerID", rid).
									Str("id", kck.ID).
									Msg("Received kick")

								candidatesLock.Lock()
								delete(candidates, kck.ID)
								candidatesLock.Unlock()
							case v1.TypeBackoff:
								log.Debug().
									Err(err).
									Str("channelID", peer.ChannelID).
									Str("peerID", rid).
									Msg("Received backoff")

								ready.Stop()

								time.Sleep(a.config.Kicks)

								greet()

								ready.Reset(a.config.Kicks)
							case v1.TypeClaimed:
								var clm v1.Claimed
								if err := mapstructure.Decode(j, &clm); err != nil {
									log.Debug().
										Err(err).
										Str("channelID", peer.ChannelID).
										Str("peerID", rid).
										Msg("Could not decode claimed from peer, stopping")

									continue
								}

								log.Debug().
									Err(err).
									Str("channelID", peer.ChannelID).
									Str("peerID", rid).
									Str("id", clm.ID).
									Msg("Received kick")

								rid = clm.ID

								if _, ok := peers[rid]; !ok {
									log.Debug().
										Err(err).
										Str("channelID", peer.ChannelID).
										Str("peerID", rid).
										Str("id", clm.ID).
										Msg("Connected to peer")
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
								log.Debug().
									Str("channelID", peer.ChannelID).
									Str("peerID", rid).
									Str("type", message.Type).
									Msg("Got message with unknown type from peer, continuing")

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

// Close disconnects the adapter from the signaler
func (a *NamedAdapter) Close() error {
	log.Trace().Msg("Closing adapter")

	return a.adapter.Close()
}

// Err returns a channel on which all fatal errors will be sent
func (a *NamedAdapter) Err() chan error {
	return a.errs
}

// Accept returns a channel on which peers will be sent when they connect
func (a *NamedAdapter) Accept() chan *Peer {
	return a.acceptedPeers
}
