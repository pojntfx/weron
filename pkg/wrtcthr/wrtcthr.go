package wrtcthr

import (
	"context"
	"crypto/rand"
	"log"
	"math"
	"strings"
	"time"

	"github.com/pojntfx/weron/pkg/services"
	"github.com/pojntfx/weron/pkg/wrtcconn"
	"github.com/teivah/broadcast"
)

const (
	acklen = 100
)

type AdapterConfig struct {
	*wrtcconn.AdapterConfig
	OnSignalerConnect  func(string)
	OnPeerConnect      func(string)
	OnPeerDisconnected func(string)
	Server             bool
	PacketCount        int
	PacketLength       int
}

type Totals struct {
	ThroughputAverageMB float64
	ThroughputAverageMb float64

	TransferredMB       int
	TransferredDuration time.Duration

	ThroughputMin float64
	ThroughputMax float64
}

type Acknowledgement struct {
	ThroughputMB float64
	ThroughputMb float64

	TransferredMB       int
	TransferredDuration time.Duration
}

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

func (a *Adapter) Open() error {
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

func (a *Adapter) Close() error {
	a.closer.Close()

	return a.adapter.Close()
}

func (a *Adapter) Wait() error {
	errs := make(chan error)

	for {
		select {
		case <-a.ctx.Done():
			if err := a.ctx.Err(); err != context.Canceled {
				return err
			}

			return nil
		case err := <-errs:
			return err
		case id := <-a.ids:
			if a.config.OnSignalerConnect != nil {
				a.config.OnSignalerConnect(id)
			}
		case peer := <-a.adapter.Accept():
			if a.config.Server {
				go func() {
					defer func() {
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
								if a.config.Verbose {
									log.Println("Could not read from peer, stopping")
								}

								return
							}

							read += n
						}

						if _, err := peer.Conn.Write(make([]byte, acklen)); err != nil {
							if a.config.Verbose {
								log.Println("Could not write to peer, stopping")
							}

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
								if a.config.Verbose {
									log.Println("Could not write to peer, stopping")
								}

								return
							}

							written += n
						}

						buf := make([]byte, acklen)
						if _, err := peer.Conn.Read(buf); err != nil {
							if a.config.Verbose {
								log.Println("Could not read from peer, stopping")
							}

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

func (a *Adapter) GatherTotals() {
	a.closer.NotifyCtx(a.ctx, struct{}{})
}

func (a *Adapter) Totals() chan Totals {
	return a.totals
}

func (a *Adapter) Acknowledgement() chan Acknowledgement {
	return a.acknowledgements
}
