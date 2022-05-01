package wrtceth

import (
	"context"
	"log"
	"runtime"
	"strings"
	"sync"

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

type AdapterConfig struct {
	*wrtcconn.AdapterConfig
	Device             string
	OnSignalerConnect  func(string)
	OnPeerConnect      func(string)
	OnPeerDisconnected func(string)
	Parallel           int
}

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

func (a *Adapter) Open() error {
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

func (a *Adapter) Close() error {
	if err := a.tap.Close(); err != nil {
		return err
	}

	return a.adapter.Close()
}

func (a *Adapter) Wait() error {
	peers := map[string]*wrtcconn.Peer{}
	var peersLock sync.Mutex

	go func() {
		sem := semaphore.NewWeighted(int64(a.config.Parallel))

		for {
			buf := make([]byte, a.mtu+ethernetHeaderLength)

			if _, err := a.tap.Read(buf); err != nil {
				if a.config.Verbose {
					log.Println("Could not read from TAP device, skipping")
				}

				continue
			}

			go func() {
				if err := sem.Acquire(a.ctx, 1); err != nil {
					if a.config.Verbose {
						log.Println("Could not acquire semaphore, skipping")
					}

					return
				}
				defer sem.Release(1)

				var frame layers.Ethernet
				if err := frame.DecodeFromBytes(buf, gopacket.NilDecodeFeedback); err != nil {
					if a.config.Verbose {
						log.Println("Could not unmarshal frame, skipping")
					}

					return
				}

				peersLock.Lock()
				for _, peer := range peers {
					// Send if matching destination, multicast or broadcast MAC
					if dst := frame.DstMAC.String(); dst == peer.PeerID || frame.DstMAC[1]&0b01 == 1 || dst == broadcastMAC {
						if _, err := peer.Conn.Write(buf); err != nil {
							if a.config.Verbose {
								log.Println("Could not write to peer, skipping")
							}

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
			if err := a.ctx.Err(); err != context.Canceled {
				return err
			}

			return nil
		case id := <-a.ids:
			if a.config.OnSignalerConnect != nil {
				a.config.OnSignalerConnect(id)
			}

			if err := setLinkUp(a.tap.Name()); err != nil {
				return err
			}
		case peer := <-a.adapter.Accept():
			if a.config.OnPeerConnect != nil {
				a.config.OnPeerConnect(peer.PeerID)
			}

			go func() {
				defer func() {
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
						if a.config.Verbose {
							log.Println("Could not read from peer, stopping")
						}

						return
					}

					if _, err := a.tap.Write(buf); err != nil {
						if a.config.Verbose {
							log.Println("Could not write to TAP device, skipping")
						}

						continue
					}
				}
			}()
		}
	}
}
