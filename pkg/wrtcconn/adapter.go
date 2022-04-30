package wrtcconn

import (
	"context"
	"errors"
	"io"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	websocketapi "github.com/pojntfx/weron/internal/api/websocket"
	"github.com/pojntfx/weron/internal/encryption"
)

var (
	ErrInvalidTURNServerAddr   = errors.New("invalid TURN server address")
	ErrMissingTURNCredentials  = errors.New("missing TURN server credentials")
	ErrMissingForcedTURNServer = errors.New("TURN is forced, but no TURN server has been configured")
)

type peer struct {
	conn       *webrtc.PeerConnection
	candidates chan webrtc.ICECandidateInit
	channels   map[string]*webrtc.DataChannel
	iid        string
}

type Peer struct {
	PeerID    string
	ChannelID string
	Conn      io.ReadWriteCloser
}

type AdapterConfig struct {
	Timeout             time.Duration
	Verbose             bool
	ID                  string
	ForceRelay          bool
	OnSignalerReconnect func()
}

type Adapter struct {
	signaler string
	key      string
	ice      []string
	channels []string
	config   *AdapterConfig
	ctx      context.Context

	cancel context.CancelFunc
	done   bool
	lines  chan []byte

	peers chan *Peer

	api *webrtc.API
}

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
			Verbose:    false,
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
						if a.config.Verbose {
							log.Println("closed connection to signaler with address", u.String()+":", err, "(wrong username or password?)")
						}
					}

					if a.config.Verbose {
						log.Println("Reconnecting to signaler with address", u.String(), "in", a.config.Timeout)
					}

					if a.config.OnSignalerReconnect != nil {
						a.config.OnSignalerReconnect()
					}

					time.Sleep(a.config.Timeout)
				}()

				ctx, cancel := context.WithTimeout(a.ctx, a.config.Timeout)
				defer cancel()

				conn, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), nil)
				if err != nil {
					panic(err)
				}

				defer func() {
					if a.config.Verbose {
						log.Println("Disconnected from signaler with address", u.String())
					}

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

				if a.config.Verbose {
					log.Println("Connected to signaler with address", u.String())
				}

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

					a.lines <- p

					if a.config.Verbose {
						log.Println("Introduced to signaler with address", u.String(), "and ID", id)
					}
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
							if a.config.Verbose {
								log.Println("Could not decrypt message with length", len(input), "for signaler with address", conn.RemoteAddr(), "in community", community+", skipping")
							}

							continue
						}

						if a.config.Verbose {
							log.Println("Received message with length", len(input), "from signaler with address", conn.RemoteAddr(), "in community", community)
						}

						var message websocketapi.Message
						if err := json.Unmarshal(input, &message); err != nil {
							if a.config.Verbose {
								log.Println("Could not unmarshal message for signaler with address", conn.RemoteAddr(), "in community", community+", skipping")
							}

							continue
						}

						switch message.Type {
						case websocketapi.TypeIntroduction:
							var introduction websocketapi.Introduction
							if err := json.Unmarshal(input, &introduction); err != nil {
								if a.config.Verbose {
									log.Println("Could not unmarshal introduction for signaler with address", conn.RemoteAddr(), "in community", community+", skipping")
								}

								continue
							}

							if a.config.Verbose {
								log.Println("Received introduction", introduction, "from signaler with address", conn.RemoteAddr(), "in community", community)
							}

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
									if a.config.Verbose {
										log.Println("Disconnected from peer", introduction.From)
									}

									peerLock.Lock()
									defer peerLock.Unlock()

									c, ok := peers[introduction.From]

									if !ok {
										if a.config.Verbose {
											log.Println("Could not find connection for peer", introduction.From, ", skipping")
										}

										return
									}

									if c.iid != iid {
										if a.config.Verbose {
											log.Println("Peer", introduction.From, ", already rejoined, not disconnecting")
										}

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
									if a.config.Verbose {
										log.Println("Created ICE candidate", i, "for signaler with address", conn.RemoteAddr(), "in community", community)
									}

									p, err := json.Marshal(websocketapi.NewCandidate(id, introduction.From, []byte(i.ToJSON().Candidate)))
									if err != nil {
										panic(err)
									}

									go func() {
										a.lines <- p

										if a.config.Verbose {
											log.Println("Sent candidate to signaler with address", u.String(), "and ID", id, "to client", introduction.From)
										}
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

								if a.config.Verbose {
									log.Println("Created data channel with ID", channelID, " using signaler with address", conn.RemoteAddr(), "in community", community)
								}

								dc.OnOpen(func() {
									if a.config.Verbose {
										log.Println("Connected to channel", dc.Label(), "with peer", introduction.From)
									}

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
									if a.config.Verbose {
										log.Println("Disconnected from channel", dc.Label(), "with peer", introduction.From)
									}

									peerLock.Lock()
									defer peerLock.Unlock()
									peer, ok := peers[introduction.From]
									if !ok {
										if a.config.Verbose {
											log.Println("Could not find peer", introduction.From, ", skipping")

										}

										return
									}

									channel, ok := peer.channels[dc.Label()]
									if !ok {
										if a.config.Verbose {
											log.Println("Could not find channel", dc.Label(), "for peer", introduction.From, ", skipping")

										}

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
										if a.config.Verbose {
											log.Println("Disconnected from peer", introduction.From)
										}

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
										a.lines <- p

										if a.config.Verbose {
											log.Println("Sent offer to signaler with address", u.String(), "and ID", id, "to client", introduction.From)
										}
									}()
								}
							}

						case websocketapi.TypeOffer:
							var offer websocketapi.Exchange
							if err := json.Unmarshal(input, &offer); err != nil {
								if a.config.Verbose {
									log.Println("Could not unmarshal offer for signaler with address", conn.RemoteAddr(), "in community", community+", skipping")
								}

								continue
							}

							if a.config.Verbose {
								log.Println("Received offer", offer, "from signaler with address", conn.RemoteAddr(), "in community", community)
							}

							if offer.To != id {
								if a.config.Verbose {
									log.Println("Discarding offer", offer, "from signaler with address", conn.RemoteAddr(), "in community", community, "because it is not intended for this client")
								}

								continue
							}

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
									if a.config.Verbose {
										log.Println("Disconnected from peer", offer.From)
									}

									peerLock.Lock()
									defer peerLock.Unlock()

									c, ok := peers[offer.From]
									if !ok {
										if a.config.Verbose {
											log.Println("Could not find connection for peer", offer.From, ", skipping")
										}

										return
									}

									if c.iid != iid {
										if a.config.Verbose {
											log.Println("Peer", offer.From, ", already rejoined, not disconnecting")
										}

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
									if a.config.Verbose {
										log.Println("Created ICE candidate", i, "for signaler with address", conn.RemoteAddr(), "in community", community)
									}

									p, err := json.Marshal(websocketapi.NewCandidate(id, offer.From, []byte(i.ToJSON().Candidate)))
									if err != nil {
										panic(err)
									}

									go func() {
										a.lines <- p

										if a.config.Verbose {
											log.Println("Sent candidate to signaler with address", u.String(), "and ID", id, "to client", offer.From)
										}
									}()
								}
							})

							c.OnDataChannel(func(dc *webrtc.DataChannel) {
								dc.OnOpen(func() {
									if a.config.Verbose {
										log.Println("Connected to channel", dc.Label(), "with peer", offer.From)
									}

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
									if a.config.Verbose {
										log.Println("Disconnected from channel", dc.Label(), "with peer", offer.From)
									}

									peerLock.Lock()
									defer peerLock.Unlock()
									channel, ok := peers[offer.From].channels[dc.Label()]
									if !ok {
										if a.config.Verbose {
											log.Println("Could not find channel", dc.Label(), "for peer", offer.From, ", skipping")

										}

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
								if a.config.Verbose {
									log.Println("Could not unmarshal SDP for signaler with address", conn.RemoteAddr(), "in community", community+", skipping")
								}

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

									if a.config.Verbose {
										log.Println("Added ICE candidate from signaler with address", u.String(), "and ID", id, "from client", offer.From)
									}
								}
							}()

							go func() {
								a.lines <- p

								if a.config.Verbose {
									log.Println("Sent answer to signaler with address", u.String(), "and ID", id, "to client", offer.From)
								}
							}()
						case websocketapi.TypeCandidate:
							var candidate websocketapi.Exchange
							if err := json.Unmarshal(input, &candidate); err != nil {
								if a.config.Verbose {
									log.Println("Could not unmarshal candidate for signaler with address", conn.RemoteAddr(), "in community", community+", skipping")
								}

								continue
							}

							if a.config.Verbose {
								log.Println("Received candidate from signaler with address", conn.RemoteAddr(), "in community", community)
							}

							if candidate.To != id {
								if a.config.Verbose {
									log.Println("Discarding candidate from signaler with address", conn.RemoteAddr(), "in community", community, "because it is not intended for this client")
								}

								continue
							}

							peerLock.Lock()
							c, ok := peers[candidate.From]

							if !ok {
								if a.config.Verbose {
									log.Println("Could not find connection for peer", candidate.From, ", skipping")
								}

								peerLock.Unlock()

								continue
							}

							go func() {
								defer func() {
									if err := recover(); err != nil {
										if a.config.Verbose {
											log.Println("Gathering candidates has stopped, skipping candidate")
										}
									}
								}()

								c.candidates <- webrtc.ICECandidateInit{Candidate: string(candidate.Payload)}
							}()

							peerLock.Unlock()
						case websocketapi.TypeAnswer:
							var answer websocketapi.Exchange
							if err := json.Unmarshal(input, &answer); err != nil {
								if a.config.Verbose {
									log.Println("Could not unmarshal answer for signaler with address", conn.RemoteAddr(), "in community", community+", skipping")
								}

								continue
							}

							if a.config.Verbose {
								log.Println("Received answer", answer, "from signaler with address", conn.RemoteAddr(), "in community", community)
							}

							if answer.To != id {
								if a.config.Verbose {
									log.Println("Discarding answer", answer, "from signaler with address", conn.RemoteAddr(), "in community", community, "because it is not intended for this client")
								}

								continue
							}

							peerLock.Lock()
							c, ok := peers[answer.From]
							peerLock.Unlock()

							if !ok {
								if a.config.Verbose {
									log.Println("Could not find connection for peer", answer.From, ", skipping")
								}

								continue
							}

							var sdp webrtc.SessionDescription
							if err := json.Unmarshal(answer.Payload, &sdp); err != nil {
								if a.config.Verbose {
									log.Println("Could not unmarshal SDP for signaler with address", conn.RemoteAddr(), "in community", community+", skipping")
								}

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

									if a.config.Verbose {
										log.Println("Added ICE candidate from signaler with address", u.String(), "and ID", id, "from client", answer.From)
									}
								}
							}()

							if a.config.Verbose {
								log.Println("Added answer from signaler with address", u.String(), "and ID", id, "from client", answer.From)
							}
						default:
							if a.config.Verbose {
								log.Println("Got message with unknown type", message.Type, "for signaler with address", conn.RemoteAddr(), "in community", community+", skipping")
							}

							continue
						}
					case line := <-a.lines:
						line, err = encryption.Encrypt(line, []byte(a.key))
						if err != nil {
							panic(err)
						}

						if a.config.Verbose {
							log.Println("Sending message with length", len(line), "to signaler with address", conn.RemoteAddr(), "in community", community)
						}

						if err := conn.WriteMessage(websocket.TextMessage, line); err != nil {
							panic(err)
						}

						if err := conn.SetWriteDeadline(time.Now().Add(a.config.Timeout)); err != nil {
							panic(err)
						}
					case <-pings.C:
						if a.config.Verbose {
							log.Println("Sending ping to signaler with address", conn.RemoteAddr(), "in community", community)
						}

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

func (a *Adapter) Close() error {
	a.done = true

	a.cancel()

	close(a.lines)

	return nil
}

func (a *Adapter) Accept() chan *Peer {
	return a.peers
}
