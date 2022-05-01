package wrtcchat

import (
	"bufio"
	"context"
	"log"
	"strings"

	"github.com/pojntfx/weron/pkg/wrtcconn"
	"github.com/teivah/broadcast"
)

type Message struct {
	PeerID    string
	ChannelID string
	Body      []byte
}

type AdapterConfig struct {
	*wrtcconn.NamedAdapterConfig
	OnSignalerConnect  func(string)
	OnPeerConnect      func(peerID string, channelID string)
	OnPeerDisconnected func(peerID string, channelID string)
	OnMessage          func(Message)
	Channels           []string
}

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

func (a *Adapter) Open() error {
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

func (a *Adapter) Close() error {
	a.input.Close()

	return a.adapter.Close()
}

func (a *Adapter) Wait() error {
	for {
		select {
		case <-a.ctx.Done():
			if err := a.ctx.Err(); err != context.Canceled {
				return err
			}

			return nil
		case id := <-a.ids:
			if a.config.OnSignalerConnect != nil {
				a.config.OnSignalerConnect(id)
			}
		case peer := <-a.adapter.Accept():
			l := a.input.Listener(0)

			if a.config.OnPeerConnect != nil {
				a.config.OnPeerConnect(peer.PeerID, peer.ChannelID)
			}

			go func() {
				defer func() {
					if a.config.OnPeerDisconnected != nil {
						a.config.OnPeerDisconnected(peer.PeerID, peer.ChannelID)
					}

					l.Close()
				}()

				reader := bufio.NewScanner(peer.Conn)
				for reader.Scan() {
					a.config.OnMessage(
						Message{
							PeerID:    peer.PeerID,
							ChannelID: peer.ChannelID,
							Body:      reader.Bytes(),
						},
					)
				}
			}()

			go func() {
				for msg := range l.Ch() {
					if _, err := peer.Conn.Write(msg); err != nil {
						if a.config.Verbose {
							log.Println("Could not write to peer, stopping")
						}

						return
					}
				}
			}()
		}
	}
}

func (a *Adapter) SendMessage(body []byte) {
	a.input.NotifyCtx(a.ctx, body)
}
