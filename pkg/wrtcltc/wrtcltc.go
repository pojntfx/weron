package wrtcltc

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

type AdapterConfig struct {
	*wrtcconn.AdapterConfig
	OnSignalerConnect  func(string)
	OnPeerConnect      func(string)
	OnPeerDisconnected func(string)
	Server             bool
	PacketLength       int
	Pause              time.Duration
}

type Totals struct {
	LatencyAverage time.Duration
	LatencyMin     time.Duration
	LatencyMax     time.Duration
	PacketsWritten int64
}

type Acknowledgement struct {
	BytesWritten int
	Latency      time.Duration
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

func (a *Adapter) Close() error {
	a.closer.Close()

	return a.adapter.Close()
}

func (a *Adapter) Wait() error {
	errs := make(chan error)

	for {
		select {
		case <-a.ctx.Done():
			return a.ctx.Err()
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
						buf := make([]byte, a.config.PacketLength)
						if _, err := peer.Conn.Read(buf); err != nil {
							if a.config.Verbose {
								log.Println("Could not read from peer, stopping")
							}

							return
						}

						if _, err := peer.Conn.Write(buf); err != nil {
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
							if a.config.Verbose {
								log.Println("Could not write to peer, stopping")
							}

							return
						}

						if _, err := peer.Conn.Read(buf); err != nil {
							if a.config.Verbose {
								log.Println("Could not read from peer, stopping")
							}

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

func (a *Adapter) GatherTotals() {
	a.closer.NotifyCtx(a.ctx, struct{}{})
}

func (a *Adapter) Totals() chan Totals {
	return a.totals
}

func (a *Adapter) Acknowledgement() chan Acknowledgement {
	return a.acknowledgements
}
