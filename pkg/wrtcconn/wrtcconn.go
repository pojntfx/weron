package wrtcconn

import (
	"context"
	"encoding/json"
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
	websocketapi "github.com/pojntfx/webrtcfd/internal/api/websocket"
	"github.com/pojntfx/webrtcfd/internal/encryption"
)

var (
	ErrInvalidTURNServerAddr  = errors.New("invalid TURN server address")
	ErrMissingTURNCredentials = errors.New("missing TURN server credentials")
)

const (
	dataChannelName = "webrtcfd"
)

type peer struct {
	conn       *webrtc.PeerConnection
	candidates chan webrtc.ICECandidateInit
	channel    *webrtc.DataChannel
}

type peerWithID struct {
	*peer
	id string
}

type AdapterConfig struct {
	Timeout time.Duration
	Verbose bool
}

type Adapter struct {
	signaler string
	key      string
	ice      []string
	config   *AdapterConfig
	ctx      context.Context

	cancel context.CancelFunc
	done   bool
	peers  chan *peerWithID
	lines  chan []byte
}

func NewAdapter(
	signaler string,
	key string,
	ice []string,
	config *AdapterConfig,
	ctx context.Context,
) *Adapter {
	ictx, cancel := context.WithCancel(ctx)

	return &Adapter{
		signaler: signaler,
		key:      key,
		ice:      ice,
		config:   config,
		ctx:      ictx,

		cancel: cancel,
		peers:  make(chan *peerWithID),
		lines:  make(chan []byte),
	}
}

func (a *Adapter) Open() (chan string, error) {
	ids := make(chan string)

	u, err := url.Parse(a.signaler)
	if err != nil {
		return ids, err
	}

	community := u.Query().Get("community")

	iceServers := []webrtc.ICEServer{}

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
		}
	}

	peers := map[string]*peer{}
	var peerLock sync.Mutex

	go func() {
		for {
			if a.done {
				return
			}

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

					for _, peer := range peers {
						if err := peer.conn.Close(); err != nil {
							panic(err)
						}

						close(peer.candidates)
					}
				}()

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

				id := uuid.New().String()

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

							c, err := webrtc.NewPeerConnection(webrtc.Configuration{
								ICEServers: iceServers,
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

									if err := c.channel.Close(); err != nil {
										panic(err)
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

							dc, err := c.CreateDataChannel(dataChannelName, nil)
							if err != nil {
								panic(err)
							}

							if a.config.Verbose {
								log.Println("Created data channel using signaler with address", conn.RemoteAddr(), "in community", community)
							}

							pr := &peer{c, make(chan webrtc.ICECandidateInit), dc}

							dc.OnOpen(func() {
								if a.config.Verbose {
									log.Println("Connected to peer", introduction.From)
								}

								a.peers <- &peerWithID{pr, introduction.From}
							})

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

							peerLock.Lock()
							peers[introduction.From] = pr
							peerLock.Unlock()

							go func() {
								a.lines <- p

								if a.config.Verbose {
									log.Println("Sent offer to signaler with address", u.String(), "and ID", id, "to client", introduction.From)
								}
							}()
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

							c, err := webrtc.NewPeerConnection(webrtc.Configuration{
								ICEServers: iceServers,
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

									if err := c.channel.Close(); err != nil {
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
										log.Println("Connected to peer", offer.From)
									}

									peerLock.Lock()
									peers[offer.From].channel = dc
									a.peers <- &peerWithID{peers[offer.From], offer.From}
									peerLock.Unlock()
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
							peers[offer.From] = &peer{c, candidates, nil}

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
								log.Println("Received candidate", candidate, "from signaler with address", conn.RemoteAddr(), "in community", community)
							}

							if candidate.To != id {
								if a.config.Verbose {
									log.Println("Discarding candidate", candidate, "from signaler with address", conn.RemoteAddr(), "in community", community, "because it is not intended for this client")
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

func (a *Adapter) Accept() (string, io.ReadWriteCloser, error) {
	p := <-a.peers

	return p.id, newDataChannelReadWriteCloser(p.channel), nil
}

type dataChannelReadWriteCloser struct {
	dc   *webrtc.DataChannel
	msgs chan []byte
}

func newDataChannelReadWriteCloser(
	dc *webrtc.DataChannel,
) *dataChannelReadWriteCloser {
	d := &dataChannelReadWriteCloser{dc, make(chan []byte)}

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		d.msgs <- msg.Data
	})

	return d
}

func (d *dataChannelReadWriteCloser) Read(p []byte) (n int, err error) {
	return copy(p, <-d.msgs), nil
}
func (d *dataChannelReadWriteCloser) Write(p []byte) (n int, err error) {
	if err := d.dc.Send(p); err != nil {
		return -1, err
	}

	return len(p), nil
}
func (d *dataChannelReadWriteCloser) Close() error {
	return d.dc.Close()
}
