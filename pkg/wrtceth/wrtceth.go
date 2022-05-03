package wrtceth

import (
	"context"
	"runtime"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/pojntfx/weron/pkg/services"
	"github.com/pojntfx/weron/pkg/wrtcconn"
	"github.com/songgao/water"
	"golang.org/x/sync/semaphore"
)

const (
	broadcastMAC         = "ff:ff:ff:ff:ff:ff"
	ethernetHeaderLength = 14
)

// AdapterConfig configures the adapter
type AdapterConfig struct {
	*wrtcconn.AdapterConfig
	Device             string       // Name to give to the TAP device
	OnSignalerConnect  func(string) // Handler to be called when the adapter has connected to the signaler
	OnPeerConnect      func(string) // Handler to be called when the adapter has connected to a peer
	OnPeerDisconnected func(string) // Handler to be called when the adapter has received a message
	Parallel           int          // Maximum amount of goroutines to use to unmarshal ethernet frames
}

// Adapter provides an ethernet service
type Adapter struct {
	signaler string
	key      string
	ice      []string
	config   *AdapterConfig
	ctx      context.Context

	cancel  context.CancelFunc
	adapter *wrtcconn.Adapter
	tap     *water.Interface
	mtu     int
	ids     chan string
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

	if config.Parallel <= 0 {
		config.Parallel = runtime.NumCPU()
	}

	return &Adapter{
		signaler: signaler,
		key:      key,
		ice:      ice,
		config:   config,
		ctx:      ictx,

		cancel: cancel,
		ids:    make(chan string),
	}
}

// Open connects the adapter to the signaler and creates the TAP device
func (a *Adapter) Open() error {
	log.Trace().Msg("Opening adapter")

	var err error
	a.tap, err = water.New(water.Config{
		DeviceType:             water.TAP,
		PlatformSpecificParams: getPlatformSpecificParams(a.config.Device),
	})
	if err != nil {
		return err
	}

	a.config.AdapterConfig.ID, err = setMACAddress(a.tap.Name(), a.config.ID)
	if err != nil {
		return err
	}

	a.adapter = wrtcconn.NewAdapter(
		a.signaler,
		a.key,
		strings.Split(strings.Join(a.ice, ","), ","),
		[]string{services.EthernetPrimary},
		a.config.AdapterConfig,
		a.ctx,
	)

	a.ids, err = a.adapter.Open()
	if err != nil {
		return err
	}

	a.mtu, err = getMTU(a.tap.Name())

	return err
}

// Close disconnects the adapter from the signaler and closes the TAP device
func (a *Adapter) Close() error {
	log.Trace().Msg("Closing adapter")

	if err := a.tap.Close(); err != nil {
		return err
	}

	return a.adapter.Close()
}

// Wait starts the transmission loop
func (a *Adapter) Wait() error {
	peers := map[string]*wrtcconn.Peer{}
	var peersLock sync.Mutex

	go func() {
		sem := semaphore.NewWeighted(int64(a.config.Parallel))

		for {
			buf := make([]byte, a.mtu+ethernetHeaderLength)

			if _, err := a.tap.Read(buf); err != nil {
				log.Debug().Err(err).Msg("Could not read from TAP device, continuing")

				continue
			}

			go func() {
				if err := sem.Acquire(a.ctx, 1); err != nil {
					log.Debug().Err(err).Msg("Could not acquire semaphore, stopping")

					return
				}
				defer sem.Release(1)

				var frame layers.Ethernet
				if err := frame.DecodeFromBytes(buf, gopacket.NilDecodeFeedback); err != nil {
					log.Debug().Err(err).Msg("Could not unmarshal frame, stopping")

					return
				}

				peersLock.Lock()
				for _, peer := range peers {
					// Send if matching destination, multicast or broadcast MAC
					if dst := frame.DstMAC.String(); dst == peer.PeerID || frame.DstMAC[1]&0b01 == 1 || dst == broadcastMAC {
						if _, err := peer.Conn.Write(buf); err != nil {
							log.Debug().
								Err(err).
								Str("channelID", peer.ChannelID).
								Str("peerID", peer.PeerID).
								Msg("Could not write to peer, continuing")

							continue
						}
					}
				}
				peersLock.Unlock()
			}()
		}
	}()

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

			if err := setLinkUp(a.tap.Name()); err != nil {
				return err
			}
		case peer := <-a.adapter.Accept():
			log.Debug().Str("channelID", peer.ChannelID).Str("peerID", peer.PeerID).Msg("Connected to peer")

			if a.config.OnPeerConnect != nil {
				a.config.OnPeerConnect(peer.PeerID)
			}

			go func() {
				defer func() {
					log.Debug().Str("channelID", peer.ChannelID).Str("peerID", peer.PeerID).Msg("Disconnected from peer")

					if a.config.OnPeerDisconnected != nil {
						a.config.OnPeerDisconnected(peer.PeerID)
					}

					peersLock.Lock()
					delete(peers, peer.PeerID)
					peersLock.Unlock()
				}()

				peersLock.Lock()
				peers[peer.PeerID] = peer
				peersLock.Unlock()

				for {
					buf := make([]byte, a.mtu+ethernetHeaderLength)

					if _, err := peer.Conn.Read(buf); err != nil {
						log.Debug().
							Err(err).
							Str("channelID", peer.ChannelID).
							Str("peerID", peer.PeerID).
							Msg("Could not read from peer, stopping")

						return
					}

					if _, err := a.tap.Write(buf); err != nil {
						log.Debug().
							Err(err).
							Str("channelID", peer.ChannelID).
							Str("peerID", peer.PeerID).
							Msg("Could not write to TAP device, continuing")

						continue
					}
				}
			}()
		}
	}
}
