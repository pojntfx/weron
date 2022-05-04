package wrtcthr

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

const (
	acklen = 100
)

// AdapterConfig configures the adapter
type AdapterConfig struct {
	*wrtcconn.AdapterConfig
	OnSignalerConnect  func(string) // Handler to be called when the adapter has connected to the signaler
	OnPeerConnect      func(string) // Handler to be called when the adapter has connected to a peer
	OnPeerDisconnected func(string) // Handler to be called when the adapter has received a message
	Server             bool         // Whether to act as the server
	PacketLength       int          // Length of the packet to measure latency with
	PacketCount        int          // Amount of packets to send before measuring
}

// Totals are the total statistics
type Totals struct {
	ThroughputAverageMB float64 // Average total throughput in megabyte/s
	ThroughputAverageMb float64 // Average total throughput in megabit/s

	TransferredMB       int           // Total transfered amount in megabyte
	TransferredDuration time.Duration // Total duration of transfer

	ThroughputMin float64 // Minimum measured throughput
	ThroughputMax float64 // Maximum measured throughput
}

// Acknowledgement is an individual datapoint
type Acknowledgement struct {
	ThroughputMB float64 // Average throughput in megabyte/s at this datapoint
	ThroughputMb float64 // Average throughput in megabit/s at this datapoint

	TransferredMB       int           // Transfered amount in megabyte at this datapoint
	TransferredDuration time.Duration // Duration of transfer at this datapoint
}

// Adapter provides a throughput measurement service
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
		[]string{services.ThroughputPrimary},
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
						read := 0
						for i := 0; i < a.config.PacketCount; i++ {
							buf := make([]byte, a.config.PacketLength)

							n, err := peer.Conn.Read(buf)
							if err != nil {
								log.Debug().
									Err(err).
									Str("channelID", peer.ChannelID).
									Str("peerID", peer.PeerID).
									Msg("Could not read from peer, stopping")

								return
							}

							read += n
						}

						if _, err := peer.Conn.Write(make([]byte, acklen)); err != nil {
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

					totalTransferred := 0
					totalStart := time.Now()

					minSpeed := math.MaxFloat64
					maxSpeed := float64(0)

					printTotals := func() {
						if totalTransferred >= 1 {
							totalDuration := time.Since(totalStart)

							totalSpeed := (float64(totalTransferred) / totalDuration.Seconds()) / 1000000

							a.totals <- Totals{
								ThroughputAverageMB: totalSpeed,
								ThroughputAverageMb: totalSpeed * 8,
								TransferredMB:       totalTransferred / 1000000,
								TransferredDuration: totalDuration,
								ThroughputMin:       minSpeed,
								ThroughputMax:       maxSpeed,
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

						written := 0
						for i := 0; i < a.config.PacketCount; i++ {
							buf := make([]byte, a.config.PacketLength)
							if _, err := rand.Read(buf); err != nil {
								errs <- err

								return
							}

							n, err := peer.Conn.Write(buf)
							if err != nil {
								log.Debug().
									Err(err).
									Str("channelID", peer.ChannelID).
									Str("peerID", peer.PeerID).
									Msg("Could not write to peer, stopping")

								return
							}

							written += n
						}

						buf := make([]byte, acklen)
						if _, err := peer.Conn.Read(buf); err != nil {
							log.Debug().
								Err(err).
								Str("channelID", peer.ChannelID).
								Str("peerID", peer.PeerID).
								Msg("Could not read from peer, stopping")

							return
						}

						duration := time.Since(start)

						speed := (float64(written) / duration.Seconds()) / 1000000

						if speed < float64(minSpeed) {
							minSpeed = speed
						}

						if speed > float64(maxSpeed) {
							maxSpeed = speed
						}

						a.acknowledgements <- Acknowledgement{
							ThroughputMB:        speed,
							ThroughputMb:        speed * 8,
							TransferredMB:       written / 1000000,
							TransferredDuration: duration,
						}

						totalTransferred += written
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
