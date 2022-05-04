package wrtcltc

import (
	"context"
	"crypto/rand"
	"math"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/pojntfx/weron/pkg/services"
	"github.com/pojntfx/weron/pkg/wrtcconn"
	"github.com/teivah/broadcast"
)

// AdapterConfig configures the adapter
type AdapterConfig struct {
	*wrtcconn.AdapterConfig
	OnSignalerConnect  func(string)  // Handler to be called when the adapter has connected to the signaler
	OnPeerConnect      func(string)  // Handler to be called when the adapter has connected to a peer
	OnPeerDisconnected func(string)  // Handler to be called when the adapter has received a message
	Server             bool          // Whether to act as the server
	PacketLength       int           // Length of the packet to measure latency with
	Pause              time.Duration // Amount of time to wait before measuring next latency datapoint
}

// Totals are the total statistics
type Totals struct {
	LatencyAverage time.Duration // Average total latency
	LatencyMin     time.Duration // Minimum mesured latency
	LatencyMax     time.Duration // Maximum measured latency
	PacketsWritten int64         // Count of written packets
}

// Acknowledgement is an individual datapoint
type Acknowledgement struct {
	BytesWritten int           // Count of written bytes
	Latency      time.Duration // Latency measured at this datapoint
}

// Adapter provides a latency measurement service
type Adapter struct {
	signaler string
	key      string
	ice      []string
	config   *AdapterConfig
	ctx      context.Context

	cancel  context.CancelFunc
	adapter *wrtcconn.Adapter

	ids              chan string
	totals           chan Totals
	acknowledgements chan Acknowledgement

	closer *broadcast.Relay[struct{}]
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

		ids:              make(chan string),
		totals:           make(chan Totals),
		acknowledgements: make(chan Acknowledgement),
	}
}

// Open connects the adapter to the signaler
func (a *Adapter) Open() error {
	log.Trace().Msg("Opening adapter")

	a.adapter = wrtcconn.NewAdapter(
		a.signaler,
		a.key,
		strings.Split(strings.Join(a.ice, ","), ","),
		[]string{services.LatencyPrimary},
		a.config.AdapterConfig,
		a.ctx,
	)

	var err error
	a.ids, err = a.adapter.Open()
	if err != nil {
		return err
	}

	a.closer = broadcast.NewRelay[struct{}]()

	return err
}

// Close disconnects the adapter from the signaler and stops all measurements, resulting in the totals being yielded
func (a *Adapter) Close() error {
	log.Trace().Msg("Closing adapter")

	a.closer.Close()

	return a.adapter.Close()
}

// Wait starts the transmission and measurement loop
func (a *Adapter) Wait() error {
	errs := make(chan error)

	for {
		select {
		case <-a.ctx.Done():
			log.Trace().Err(a.ctx.Err()).Msg("Context cancelled")

			if err := a.ctx.Err(); err != context.Canceled {
				return err
			}

			return nil
		case err := <-errs:
			return err
		case id := <-a.ids:
			log.Debug().Str("id", id).Msg("Connected to signaler")

			if a.config.OnSignalerConnect != nil {
				a.config.OnSignalerConnect(id)
			}
		case peer := <-a.adapter.Accept():
			log.Debug().Str("channelID", peer.ChannelID).Str("peerID", peer.PeerID).Msg("Connected to peer")

			if a.config.Server {
				go func() {
					defer func() {
						log.Debug().Str("channelID", peer.ChannelID).Str("peerID", peer.PeerID).Msg("Disconnected from peer")

						if a.config.OnPeerDisconnected != nil {
							a.config.OnPeerDisconnected(peer.PeerID)
						}
					}()

					if a.config.OnPeerConnect != nil {
						a.config.OnPeerConnect(peer.PeerID)
					}

					for {
						buf := make([]byte, a.config.PacketLength)
						if _, err := peer.Conn.Read(buf); err != nil {
							log.Debug().
								Err(err).
								Str("channelID", peer.ChannelID).
								Str("peerID", peer.PeerID).
								Msg("Could not read from peer, stopping")

							return
						}

						if _, err := peer.Conn.Write(buf); err != nil {
							log.Debug().
								Err(err).
								Str("channelID", peer.ChannelID).
								Str("peerID", peer.PeerID).
								Msg("Could not write to peer, stopping")

							return
						}
					}
				}()
			} else {
				go func() {
					if a.config.OnPeerConnect != nil {
						a.config.OnPeerConnect(peer.PeerID)
					}

					packetsWritten := int64(0)
					totalLatency := time.Duration(0)

					minLatency := time.Duration(math.MaxInt64)
					maxLatency := time.Duration(0)

					printTotals := func() {
						if packetsWritten >= 1 {
							averageLatency := totalLatency.Nanoseconds() / packetsWritten

							a.totals <- Totals{
								LatencyAverage: time.Duration(averageLatency),
								LatencyMin:     minLatency,
								LatencyMax:     maxLatency,
								PacketsWritten: packetsWritten,
							}
						}
					}

					go func() {
						c := a.closer.Listener(0)
						defer c.Close()

						<-c.Ch()

						printTotals()
					}()

					defer func() {
						printTotals()

						if a.config.OnPeerDisconnected != nil {
							a.config.OnPeerDisconnected(peer.PeerID)
						}
					}()

					for {
						start := time.Now()

						buf := make([]byte, a.config.PacketLength)
						if _, err := rand.Read(buf); err != nil {
							errs <- err

							return
						}

						written, err := peer.Conn.Write(buf)
						if err != nil {
							log.Debug().
								Err(err).
								Str("channelID", peer.ChannelID).
								Str("peerID", peer.PeerID).
								Msg("Could not write to peer, stopping")

							return
						}

						if _, err := peer.Conn.Read(buf); err != nil {
							log.Debug().
								Err(err).
								Str("channelID", peer.ChannelID).
								Str("peerID", peer.PeerID).
								Msg("Could not read from peer, stopping")

							return
						}

						latency := time.Since(start)

						if latency < minLatency {
							minLatency = latency
						}

						if latency > maxLatency {
							maxLatency = latency
						}

						totalLatency += latency
						packetsWritten++

						a.acknowledgements <- Acknowledgement{
							BytesWritten: written,
							Latency:      latency,
						}

						time.Sleep(a.config.Pause)
					}
				}()
			}
		}
	}
}

// GatherTotals yields the total statistics
func (a *Adapter) GatherTotals() {
	a.closer.NotifyCtx(a.ctx, struct{}{})
}

// Totals returns a channel on which all total statistics will be sent
func (a *Adapter) Totals() chan Totals {
	return a.totals
}

// Acknowledgements returns a channel on which all individual datapoints will be sent
func (a *Adapter) Acknowledgements() chan Acknowledgement {
	return a.acknowledgements
}
