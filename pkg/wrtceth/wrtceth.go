package wrtceth

import (
	"context"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/mdlayher/ethernet"
	"github.com/pojntfx/webrtcfd/pkg/wrtcconn"
	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
)

const (
	dataChannelName      = "webrtcfd"
	broadcastMAC         = "ff:ff:ff:ff:ff:ff"
	ethernetHeaderLength = 14
)

type AdapterConfig struct {
	*wrtcconn.AdapterConfig
	Device             string
	OnSignalerConnect  func(string)
	OnPeerConnect      func(string)
	OnPeerDisconnected func(string)
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
	link    netlink.Link
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
		DeviceType: water.TAP,
		PlatformSpecificParams: water.PlatformSpecificParams{
			Name: a.config.Device,
		},
	})
	if err != nil {
		return err
	}

	a.link, err = netlink.LinkByName(a.tap.Name())
	if err != nil {
		return err
	}

	var mac net.HardwareAddr
	if strings.TrimSpace(a.config.ID) == "" {
		mac = a.link.Attrs().HardwareAddr
	} else {
		mac, err = net.ParseMAC(a.config.ID)
		if err != nil {
			return err
		}
	}
	a.config.AdapterConfig.ID = mac.String()

	if err := netlink.LinkSetHardwareAddr(a.link, mac); err != nil {
		return err
	}

	a.adapter = wrtcconn.NewAdapter(
		a.signaler,
		a.key,
		strings.Split(strings.Join(a.ice, ","), ","),
		a.config.AdapterConfig,
		a.ctx,
	)

	a.ids, err = a.adapter.Open()

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
		for {
			buf := make([]byte, a.link.Attrs().MTU+ethernetHeaderLength)

			if _, err := a.tap.Read(buf); err != nil {
				if a.config.Verbose {
					log.Println("Could not read from TAP device, skipping")
				}

				continue
			}

			var frame ethernet.Frame
			if err := frame.UnmarshalBinary(buf); err != nil {
				if a.config.Verbose {
					log.Println("Could not unmarshal frame, skipping")
				}

				continue
			}

			peersLock.Lock()
			for _, peer := range peers {
				// Send if broadcast, multicast or matching destination MAC
				if dst := frame.Destination.String(); dst == broadcastMAC || frame.Destination[1]&0b01 == 1 || dst == peer.ID {
					if _, err := peer.Conn.Write(buf); err != nil {
						if a.config.Verbose {
							log.Println("Could not write to peer, skipping")
						}

						continue
					}
				}
			}
			peersLock.Unlock()
		}
	}()

	for {
		select {
		case <-a.ctx.Done():
			return nil
		case id := <-a.ids:
			if a.config.OnSignalerConnect != nil {
				a.config.OnSignalerConnect(id)
			}

			if err := netlink.LinkSetUp(a.link); err != nil {
				return err
			}
		case peer := <-a.adapter.Accept():
			if a.config.OnPeerConnect != nil {
				a.config.OnPeerConnect(peer.ID)
			}

			go func() {
				defer func() {
					if a.config.OnPeerDisconnected != nil {
						a.config.OnPeerDisconnected(peer.ID)
					}

					peersLock.Lock()
					delete(peers, peer.ID)
					peersLock.Unlock()
				}()

				peersLock.Lock()
				peers[peer.ID] = peer
				peersLock.Unlock()

				for {
					buf := make([]byte, a.link.Attrs().MTU+ethernetHeaderLength)

					if _, err := peer.Conn.Read(buf); err != nil {
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
