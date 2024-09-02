package wrtcconn

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	websocketapi "github.com/pojntfx/weron/internal/api/websocket"
	"github.com/pojntfx/weron/internal/encryption"
	"github.com/rs/zerolog/log"
)

var (
	ErrInvalidTURNServerAddr   = errors.New("invalid TURN server address")                            // The specified TURN server address is invalid
	ErrMissingTURNCredentials  = errors.New("missing TURN server credentials")                        // The specified TURN server is missing credentials
	ErrMissingForcedTURNServer = errors.New("TURN is forced, but no TURN server has been configured") // All connections must use TURN, but no TURN server has been configured
)

type peer struct {
	conn       *webrtc.PeerConnection
	candidates chan webrtc.ICECandidateInit
	channels   map[string]*webrtc.DataChannel
	iid        string
}

// Peer is a connected remote adapter
type Peer struct {
	PeerID    string             // ID of the peer
	ChannelID string             // Channel on which the peer is connected to
	Conn      io.ReadWriteCloser // Underlying connection to send/receive on
}

// AdapterConfig configures the adapter
type AdapterConfig struct {
	Timeout             time.Duration // Time to wait before retrying to connect to the signaler
	ID                  string        // ID to claim without conflict resolution (default is UUID)
	ForceRelay          bool          // Whether to block P2P connections
	OnSignalerReconnect func()        // Handler to be called when the adapter has reconnected to the signaler
}

// NamedAdapter provides a connection service without name conflict prevention
type Adapter struct {
	signaler string
	key      string
	ice      []string
	channels []string
	config   *AdapterConfig
	ctx      context.Context

	cancel   context.CancelFunc
	done     bool
	doneSync sync.Mutex
	lines    chan []byte

	peers chan *Peer

	api *webrtc.API
}

// NewAdapter creates the adapter
func NewAdapter(
	signaler string,
	key string,
	ice []string,
	channels []string,
	config *AdapterConfig,
	ctx context.Context,
) *Adapter {
	ictx, cancel := context.WithCancel(ctx)

	if config == nil {
		config = &AdapterConfig{
			Timeout:    time.Second * 10,
			ID:         "",
			ForceRelay: false,
		}
	}

	return &Adapter{
		signaler: signaler,
		key:      key,
		ice:      ice,
		channels: channels,
		config:   config,
		ctx:      ictx,

		cancel: cancel,
		peers:  make(chan *Peer),
		lines:  make(chan []byte),
	}
}

func (a *Adapter) sendLine(line []byte) {
	a.doneSync.Lock()
	defer a.doneSync.Unlock()

	if a.done {
		return
	}

	a.lines <- line
}

// Open connects the adapter to the signaler
func (a *Adapter) Open() (chan string, error) {
	settingEngine := webrtc.SettingEngine{}
	settingEngine.DetachDataChannels()
	a.api = webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))

	ids := make(chan string)

	u, err := url.Parse(a.signaler)
	if err != nil {
		return ids, err
	}

	community := u.Query().Get("community")

	iceServers := []webrtc.ICEServer{}

	containsTURN := false
	for _, ice := range a.ice {
		// Skip empty server configs
		if strings.TrimSpace(ice) == "" {
			log.Trace().Msg("Skipping empty server config")

			continue
		}

		if strings.Contains(ice, "stun:") {
			iceServers = append(iceServers, webrtc.ICEServer{
				URLs: []string{ice},
			})
		} else {
			addrParts := strings.Split(ice, "@")
			if len(addrParts) < 2 {
				return ids, ErrInvalidTURNServerAddr
			}

			authParts := strings.Split(addrParts[0], ":")
			if len(addrParts) < 2 {
				return ids, ErrMissingTURNCredentials
			}

			iceServers = append(iceServers, webrtc.ICEServer{
				URLs:           []string{addrParts[1]},
				Username:       authParts[0],
				Credential:     authParts[1],
				CredentialType: webrtc.ICECredentialTypePassword,
			})

			containsTURN = true
		}
	}

	if a.config.ForceRelay && !containsTURN {
		return ids, ErrMissingForcedTURNServer
	}

	go func() {
		for {
			if a.done {
				return
			}

			peers := map[string]*peer{}
			var peerLock sync.Mutex

			func() {
				defer func() {
					if err := recover(); err != nil {
						log.Debug().Str("address", u.String()).Err(err.(error)).Msg("Closed connection to signaler (wrong username or password?)")
					}

					log.Debug().Str("address", u.String()).Dur("timeout", a.config.Timeout).Msg("Reconnecting to signaler")

					if a.config.OnSignalerReconnect != nil {
						a.config.OnSignalerReconnect()
					}

					time.Sleep(a.config.Timeout)
				}()

				ctx, cancel := context.WithTimeout(a.ctx, a.config.Timeout)
				defer cancel()

				headers := http.Header{}
				if u.User != nil {
					headers.Set("Authorization", "Basic " + base64.StdEncoding.EncodeToString([]byte(u.User.String())))
					u.User = nil
				}

				conn, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), headers)
				if err != nil {
					panic(err)
				}

				defer func() {
					log.Debug().Str("address", u.String()).Msg("Disconnected from signaler")

					if err := conn.Close(); err != nil {
						panic(err)
					}

					peerLock.Lock()
					defer peerLock.Unlock()

					for _, peer := range peers {
						for _, channel := range peer.channels {
							if err := channel.Close(); err != nil {
								panic(err)
							}
						}

						if err := peer.conn.Close(); err != nil {
							panic(err)
						}

						close(peer.candidates)
					}
				}()

				if err := conn.SetReadDeadline(time.Now().Add(a.config.Timeout)); err != nil {
					panic(err)
				}
				conn.SetPongHandler(func(string) error {
					return conn.SetReadDeadline(time.Now().Add(a.config.Timeout))
				})

				log.Debug().Str("address", u.String()).Msg("Connected to signaler")

				inputs := make(chan []byte)
				errs := make(chan error)
				go func() {
					defer func() {
						close(inputs)
						close(errs)
					}()

					for {
						_, p, err := conn.ReadMessage()
						if err != nil {
							errs <- err

							return
						}

						inputs <- p
					}
				}()

				id := a.config.ID
				if strings.TrimSpace(id) == "" {
					id = uuid.New().String()
				}

				ids <- id

				go func() {
					p, err := json.Marshal(websocketapi.NewIntroduction(id))
					if err != nil {
						errs <- err

						return
					}

					a.sendLine(p)

					log.Debug().Str("address", u.String()).Str("id", id).Msg("Introduced to signaler")
				}()

				pings := time.NewTicker(a.config.Timeout / 2)
				defer pings.Stop()

				for {
					select {
					case err := <-errs:
						panic(err)
					case input := <-inputs:
						input, err = encryption.Decrypt(input, []byte(a.key))
						if err != nil {
							log.Debug().
								Str("address", conn.RemoteAddr().String()).
								Int("len", len(input)).
								Str("community", community).
								Str("id", id).Msg("Could not decrypt message from signaler, continuing")

							continue
						}

						log.Trace().
							Str("address", conn.RemoteAddr().String()).
							Int("len", len(input)).
							Str("community", community).
							Str("id", id).Msg("Received message from signaler")

						var message websocketapi.Message
						if err := json.Unmarshal(input, &message); err != nil {
							log.Debug().
								Str("address", conn.RemoteAddr().String()).
								Str("community", community).
								Str("id", id).Msg("Could not unmarshal message from signaler, continuing")

							continue
						}

						switch message.Type {
						case websocketapi.TypeIntroduction:
							var introduction websocketapi.Introduction
							if err := json.Unmarshal(input, &introduction); err != nil {
								log.Debug().
									Str("address", conn.RemoteAddr().String()).
									Str("community", community).
									Str("id", id).Msg("Could not unmarshal introduction from signaler, continuing")

								continue
							}

							log.Debug().
								Str("address", conn.RemoteAddr().String()).
								Str("community", community).
								Str("id", id).Msg("Received introduction from signaler")

							iid := uuid.NewString()

							transportPolicy := webrtc.ICETransportPolicyAll
							if a.config.ForceRelay {
								transportPolicy = webrtc.ICETransportPolicyRelay
							}

							c, err := a.api.NewPeerConnection(webrtc.Configuration{
								ICEServers:         iceServers,
								ICETransportPolicy: transportPolicy,
							})
							if err != nil {
								panic(err)
							}

							c.OnConnectionStateChange(func(pcs webrtc.PeerConnectionState) {
								if pcs == webrtc.PeerConnectionStateDisconnected {
									log.Debug().Str("peerID", introduction.From).Msg("Disconnected from peer")

									peerLock.Lock()
									defer peerLock.Unlock()

									c, ok := peers[introduction.From]

									if !ok {
										log.Debug().Str("peerID", introduction.From).Msg("Could not find connection for peer, continuing")

										return
									}

									if c.iid != iid {
										log.Debug().Str("peerID", introduction.From).Msg("Peer already rejoined, not disconnecting")

										return
									}

									for _, channel := range c.channels {
										if err := channel.Close(); err != nil {
											panic(err)
										}
									}

									if err := c.conn.Close(); err != nil {
										panic(err)
									}

									close(c.candidates)

									delete(peers, introduction.From)
								}
							})

							c.OnICECandidate(func(i *webrtc.ICECandidate) {
								if i != nil {
									log.Trace().
										Str("address", conn.RemoteAddr().String()).
										Str("len", i.String()).
										Str("community", community).
										Str("id", id).Msg("Created ICE candidate")

									p, err := json.Marshal(websocketapi.NewCandidate(id, introduction.From, []byte(i.ToJSON().Candidate)))
									if err != nil {
										panic(err)
									}

									go func() {
										a.sendLine(p)

										log.Debug().
											Str("address", conn.RemoteAddr().String()).
											Str("community", community).
											Str("id", id).
											Str("client", introduction.From).
											Msg("Sent ICE candidate to signaler")
									}()
								}
							})

							for i, channelID := range a.channels {
								// Skip empty channel IDs
								if strings.TrimSpace(channelID) == "" {
									continue
								}

								dc, err := c.CreateDataChannel(channelID, nil)
								if err != nil {
									panic(err)
								}

								log.Trace().
									Str("address", conn.RemoteAddr().String()).
									Str("community", community).
									Str("channelID", channelID).
									Msg("Created data channel")

								dc.OnOpen(func() {
									log.Debug().
										Str("label", dc.Label()).
										Str("peer", introduction.From).
										Msg("Connected to channel")

									c, err := dc.Detach()
									if err != nil {
										panic(err)
									}

									for _, channel := range a.channels {
										if dc.Label() == channel {
											peerLock.Lock()
											peers[introduction.From].channels[dc.Label()] = dc
											a.peers <- &Peer{introduction.From, dc.Label(), c}
											peerLock.Unlock()

											break
										}
									}
								})

								dc.OnClose(func() {
									log.Debug().
										Str("label", dc.Label()).
										Str("peer", introduction.From).
										Msg("Disconnected from channel")

									peerLock.Lock()
									defer peerLock.Unlock()
									peer, ok := peers[introduction.From]
									if !ok {
										log.Debug().Str("peerID", introduction.From).Msg("Could not find peer, continuing")

										return
									}

									channel, ok := peer.channels[dc.Label()]
									if !ok {
										log.Debug().
											Str("peerID", introduction.From).
											Str("channelID", dc.Label()).
											Msg("Could not find channel, continuing")

										return
									}

									if err := channel.Close(); err != nil {
										panic(err)
									}

									delete(peers[introduction.From].channels, dc.Label())
								})

								if i == 0 {
									o, err := c.CreateOffer(nil)
									if err != nil {
										panic(err)
									}

									if err := c.SetLocalDescription(o); err != nil {
										panic(err)
									}

									oj, err := json.Marshal(o)
									if err != nil {
										panic(err)
									}

									p, err := json.Marshal(websocketapi.NewOffer(id, introduction.From, oj))
									if err != nil {
										panic(err)
									}

									pr := &peer{c, make(chan webrtc.ICECandidateInit), map[string]*webrtc.DataChannel{
										dc.Label(): dc,
									}, iid}

									peerLock.Lock()
									old, ok := peers[introduction.From]
									if ok {
										// Disconnect the old peer
										log.Debug().Str("peerID", introduction.From).Msg("Disconnected from peer")

										for _, channel := range old.channels {
											if err := channel.Close(); err != nil {
												panic(err)
											}
										}

										if err := old.conn.Close(); err != nil {
											panic(err)
										}

										close(old.candidates)
									}
									peers[introduction.From] = pr
									peerLock.Unlock()

									go func() {
										a.sendLine(p)

										log.Debug().
											Str("address", conn.RemoteAddr().String()).
											Str("community", community).
											Str("id", id).
											Str("client", introduction.From).
											Msg("Sent offer to signaler")
									}()
								}
							}

						case websocketapi.TypeOffer:
							var offer websocketapi.Exchange
							if err := json.Unmarshal(input, &offer); err != nil {
								log.Debug().
									Str("address", conn.RemoteAddr().String()).
									Str("community", community).
									Str("id", id).Msg("Could not unmarshal offer from signaler, continuing")

								continue
							}

							if offer.To != id {
								log.Trace().
									Str("address", conn.RemoteAddr().String()).
									Str("community", community).
									Str("id", id).Msg("Discarding offer from signaler because it is not intended for this client")

								continue
							}

							log.Debug().
								Str("address", conn.RemoteAddr().String()).
								Str("community", community).
								Str("id", id).Msg("Received offer from signaler")

							iid := uuid.NewString()

							transportPolicy := webrtc.ICETransportPolicyAll
							if a.config.ForceRelay {
								transportPolicy = webrtc.ICETransportPolicyRelay
							}

							c, err := a.api.NewPeerConnection(webrtc.Configuration{
								ICEServers:         iceServers,
								ICETransportPolicy: transportPolicy,
							})
							if err != nil {
								panic(err)
							}

							c.OnConnectionStateChange(func(pcs webrtc.PeerConnectionState) {
								if pcs == webrtc.PeerConnectionStateDisconnected {
									log.Debug().Str("peerID", offer.From).Msg("Disconnected from peer")

									peerLock.Lock()
									defer peerLock.Unlock()

									c, ok := peers[offer.From]
									if !ok {
										log.Debug().Str("peerID", offer.From).Msg("Could not find connection for peer, continuing")

										return
									}

									if c.iid != iid {
										log.Debug().Str("peerID", offer.From).Msg("Peer already rejoined, not disconnecting")

										return
									}

									if err := c.conn.Close(); err != nil {
										panic(err)
									}

									if err := c.conn.Close(); err != nil {
										panic(err)
									}

									close(c.candidates)

									delete(peers, offer.From)
								}
							})

							c.OnICECandidate(func(i *webrtc.ICECandidate) {
								if i != nil {
									log.Trace().
										Str("address", conn.RemoteAddr().String()).
										Str("len", i.String()).
										Str("community", community).
										Str("id", id).Msg("Created ICE candidate")

									p, err := json.Marshal(websocketapi.NewCandidate(id, offer.From, []byte(i.ToJSON().Candidate)))
									if err != nil {
										panic(err)
									}

									go func() {
										a.sendLine(p)

										log.Debug().
											Str("address", conn.RemoteAddr().String()).
											Str("community", community).
											Str("id", id).
											Str("client", offer.From).
											Msg("Sent ICE candidate to signaler")
									}()
								}
							})

							c.OnDataChannel(func(dc *webrtc.DataChannel) {
								dc.OnOpen(func() {
									log.Debug().
										Str("label", dc.Label()).
										Str("peer", offer.From).
										Msg("Connected to channel")

									c, err := dc.Detach()
									if err != nil {
										panic(err)
									}

									for _, channel := range a.channels {
										if dc.Label() == channel {
											peerLock.Lock()
											peers[offer.From].channels[dc.Label()] = dc
											a.peers <- &Peer{offer.From, dc.Label(), c}
											peerLock.Unlock()

											break
										}
									}
								})

								dc.OnClose(func() {
									log.Debug().
										Str("label", dc.Label()).
										Str("peer", offer.From).
										Msg("Disconnected from channel")

									peerLock.Lock()
									defer peerLock.Unlock()
									channel, ok := peers[offer.From].channels[dc.Label()]
									if !ok {
										log.Debug().
											Str("peerID", offer.From).
											Str("channelID", dc.Label()).
											Msg("Could not find channel, continuing")

										return
									}

									if err := channel.Close(); err != nil {
										panic(err)
									}

									delete(peers[offer.From].channels, dc.Label())
								})
							})

							var sdp webrtc.SessionDescription
							if err := json.Unmarshal(offer.Payload, &sdp); err != nil {
								log.Debug().
									Str("address", conn.RemoteAddr().String()).
									Str("community", community).
									Str("id", id).Msg("Could not unmarshal SDP from signaler, continuing")

								continue
							}

							if err := c.SetRemoteDescription(sdp); err != nil {
								panic(err)
							}

							ans, err := c.CreateAnswer(nil)
							if err != nil {
								panic(err)
							}

							if err := c.SetLocalDescription(ans); err != nil {
								panic(err)
							}

							aj, err := json.Marshal(ans)
							if err != nil {
								panic(err)
							}

							p, err := json.Marshal(websocketapi.NewAnswer(id, offer.From, aj))
							if err != nil {
								panic(err)
							}

							peerLock.Lock()

							candidates := make(chan webrtc.ICECandidateInit)
							peers[offer.From] = &peer{c, candidates, map[string]*webrtc.DataChannel{}, iid}

							peerLock.Unlock()

							go func() {
								for candidate := range candidates {
									if err := c.AddICECandidate(candidate); err != nil {
										errs <- err

										return
									}

									log.Debug().
										Str("address", conn.RemoteAddr().String()).
										Str("community", community).
										Str("id", id).
										Str("peerID", offer.From).
										Msg("Added ICE candidate from signaler")
								}
							}()

							go func() {
								a.sendLine(p)

								log.Debug().
									Str("address", conn.RemoteAddr().String()).
									Str("community", community).
									Str("id", id).
									Str("client", offer.From).
									Msg("Sent answer to signaler")
							}()
						case websocketapi.TypeCandidate:
							var candidate websocketapi.Exchange
							if err := json.Unmarshal(input, &candidate); err != nil {
								log.Debug().
									Str("address", conn.RemoteAddr().String()).
									Str("community", community).
									Str("id", id).Msg("Could not unmarshal candidate from signaler, continuing")

								continue
							}

							if candidate.To != id {
								log.Trace().
									Str("address", conn.RemoteAddr().String()).
									Str("community", community).
									Str("id", id).Msg("Discarding candidate from signaler because it is not intended for this client")

								continue
							}

							log.Debug().
								Str("address", conn.RemoteAddr().String()).
								Str("community", community).
								Str("id", id).Msg("Received candidate from signaler")

							peerLock.Lock()
							c, ok := peers[candidate.From]

							if !ok {
								log.Debug().Str("peerID", candidate.From).Msg("Could not find connection for peer, continuing")

								peerLock.Unlock()

								continue
							}

							go func() {
								defer func() {
									if err := recover(); err != nil {
										log.Debug().
											Str("address", conn.RemoteAddr().String()).
											Str("community", community).
											Str("id", id).
											Msg("Gathering candiates has stopped, continuing candidate")
									}
								}()

								c.candidates <- webrtc.ICECandidateInit{Candidate: string(candidate.Payload)}
							}()

							peerLock.Unlock()
						case websocketapi.TypeAnswer:
							var answer websocketapi.Exchange
							if err := json.Unmarshal(input, &answer); err != nil {
								log.Debug().
									Str("address", conn.RemoteAddr().String()).
									Str("community", community).
									Str("id", id).Msg("Could not unmarshal answer from signaler, continuing")

								continue
							}

							if answer.To != id {
								log.Trace().
									Str("address", conn.RemoteAddr().String()).
									Str("community", community).
									Str("id", id).Msg("Discarding answer from signaler because it is not intended for this client")

								continue
							}

							log.Debug().
								Str("address", conn.RemoteAddr().String()).
								Str("community", community).
								Str("id", id).Msg("Received answer from signaler")

							peerLock.Lock()
							c, ok := peers[answer.From]
							peerLock.Unlock()

							if !ok {
								log.Debug().Str("peerID", answer.From).Msg("Could not find connection for peer, continuing")

								continue
							}

							var sdp webrtc.SessionDescription
							if err := json.Unmarshal(answer.Payload, &sdp); err != nil {
								log.Debug().
									Str("address", conn.RemoteAddr().String()).
									Str("community", community).
									Str("id", id).Msg("Could not unmarshal SDP from signaler, continuing")

								continue
							}

							if err := c.conn.SetRemoteDescription(sdp); err != nil {
								panic(err)
							}

							go func() {
								for candidate := range c.candidates {
									if err := c.conn.AddICECandidate(candidate); err != nil {
										errs <- err

										return
									}

									log.Debug().
										Str("address", conn.RemoteAddr().String()).
										Str("community", community).
										Str("id", id).
										Str("peerID", answer.From).
										Msg("Added ICE candidate from signaler")
								}
							}()

							log.Debug().
								Str("address", conn.RemoteAddr().String()).
								Str("community", community).
								Str("id", id).
								Str("peerID", answer.From).
								Msg("Added answer from signaler")
						default:
							log.Debug().
								Str("address", conn.RemoteAddr().String()).
								Str("community", community).
								Str("id", id).
								Str("type", message.Type).
								Msg("Got message with unknown type from signaler, continuing")

							continue
						}
					case line := <-a.lines:
						line, err = encryption.Encrypt(line, []byte(a.key))
						if err != nil {
							panic(err)
						}

						log.Trace().
							Str("address", conn.RemoteAddr().String()).
							Str("community", community).
							Str("id", id).
							Int("len", len(line)).
							Msg("Sending message to signaler")

						if err := conn.WriteMessage(websocket.TextMessage, line); err != nil {
							panic(err)
						}

						if err := conn.SetWriteDeadline(time.Now().Add(a.config.Timeout)); err != nil {
							panic(err)
						}
					case <-pings.C:
						log.Trace().
							Str("address", conn.RemoteAddr().String()).
							Str("community", community).
							Str("id", id).
							Msg("Sending ping to signaler")

						if err := conn.SetWriteDeadline(time.Now().Add(a.config.Timeout)); err != nil {
							panic(err)
						}

						if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
							panic(err)
						}
					}
				}
			}()
		}
	}()

	return ids, nil
}

// Close disconnects the adapter from the signaler
func (a *Adapter) Close() error {
	log.Trace().Msg("Closing adapter")

	a.done = true

	a.cancel()

	close(a.lines)

	return nil
}

// Accept returns a channel on which peers will be sent when they connect
func (a *Adapter) Accept() chan *Peer {
	return a.peers
}
