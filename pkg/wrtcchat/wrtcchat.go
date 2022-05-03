package wrtcchat

import (
	"bufio"
	"context"
	"strings"

	"github.com/pojntfx/weron/pkg/wrtcconn"
	"github.com/rs/zerolog/log"
	"github.com/teivah/broadcast"
)

// Message is a chat message
type Message struct {
	PeerID    string // ID of the peer that sent the message
	ChannelID string // Channel to which the message has been sent
	Body      []byte // Content of the message
}

// AdapterConfig configures the adapter
type AdapterConfig struct {
	*wrtcconn.NamedAdapterConfig
	OnSignalerConnect  func(string)                          // Handler to be called when the adapter has connected to the signaler
	OnPeerConnect      func(peerID string, channelID string) // Handler to be called when the adapter has connected to a peer
	OnPeerDisconnected func(peerID string, channelID string) // Handler to be called when the adapter has disconnected from a peer
	OnMessage          func(Message)                         // Handler to be called when the adapter has received a message
	Channels           []string                              // Channels to join
}

// Adapter provides a chat service
type Adapter struct {
	signaler string
	key      string
	ice      []string
	config   *AdapterConfig
	ctx      context.Context

	cancel  context.CancelFunc
	adapter *wrtcconn.NamedAdapter

	ids   chan string
	input *broadcast.Relay[[]byte]
}

// NewAdapter creates the adapter
func NewAdapter(
	signaler string,
	key string,
	ice []string,
	config *AdapterConfig,
	ctx context.Context,
) *Adapter {
	ictx, cancel := context.WithCancel(ctx)

	if config == nil {
		config = &AdapterConfig{}
	}

	return &Adapter{
		signaler: signaler,
		key:      key,
		ice:      ice,
		config:   config,
		ctx:      ictx,

		cancel: cancel,

		ids:   make(chan string),
		input: broadcast.NewRelay[[]byte](),
	}
}

// Open connects the adapter to the signaler
func (a *Adapter) Open() error {
	log.Trace().Msg("Opening adapter")

	a.adapter = wrtcconn.NewNamedAdapter(
		a.signaler,
		a.key,
		strings.Split(strings.Join(a.ice, ","), ","),
		a.config.Channels,
		a.config.NamedAdapterConfig,
		a.ctx,
	)

	var err error
	a.ids, err = a.adapter.Open()
	if err != nil {
		return err
	}

	return err
}

// Close disconnects the adapter from the signaler
func (a *Adapter) Close() error {
	log.Trace().Msg("Closing adapter")

	a.input.Close()

	return a.adapter.Close()
}

// Wait starts the transmission loop
func (a *Adapter) Wait() error {
	for {
		select {
		case <-a.ctx.Done():
			log.Trace().Err(a.ctx.Err()).Msg("Context cancelled")

			if err := a.ctx.Err(); err != context.Canceled {
				return err
			}

			return nil
		case id := <-a.ids:
			log.Debug().Str("id", id).Msg("Connected to signaler")

			if a.config.OnSignalerConnect != nil {
				a.config.OnSignalerConnect(id)
			}
		case peer := <-a.adapter.Accept():
			log.Debug().Str("channelID", peer.ChannelID).Str("peerID", peer.PeerID).Msg("Connected to peer")

			l := a.input.Listener(0)

			if a.config.OnPeerConnect != nil {
				a.config.OnPeerConnect(peer.PeerID, peer.ChannelID)
			}

			go func() {
				defer func() {
					log.Debug().Str("channelID", peer.ChannelID).Str("peerID", peer.PeerID).Msg("Disconnected from peer")

					if a.config.OnPeerDisconnected != nil {
						a.config.OnPeerDisconnected(peer.PeerID, peer.ChannelID)
					}

					l.Close()
				}()

				reader := bufio.NewScanner(peer.Conn)
				for reader.Scan() {
					body := reader.Bytes()

					log.Trace().Bytes("body", body).Msg("Received message")

					a.config.OnMessage(
						Message{
							PeerID:    peer.PeerID,
							ChannelID: peer.ChannelID,
							Body:      body,
						},
					)
				}
			}()

			go func() {
				for msg := range l.Ch() {
					if _, err := peer.Conn.Write(msg); err != nil {
						log.Debug().
							Err(err).
							Str("channelID", peer.ChannelID).
							Str("peerID", peer.PeerID).
							Msg("Could not write to peer, stopping")

						return
					}
				}
			}()
		}
	}
}

// SendMessage sends a message to all peers
func (a *Adapter) SendMessage(body []byte) {
	log.Trace().Bytes("body", body).Msg("Sending message")

	a.input.NotifyCtx(a.ctx, body)
}
