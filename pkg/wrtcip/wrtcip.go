package wrtcip

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"log"
	"net"
	"runtime"
	"strings"
	"sync"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/pojntfx/webrtcfd/pkg/wrtcconn"
	"github.com/songgao/water"
	"golang.org/x/sync/semaphore"
)

const (
	headerLength = 22
)

type AdapterConfig struct {
	*wrtcconn.AdapterConfig
	Device             string
	OnSignalerConnect  func(string)
	OnPeerConnect      func(string)
	OnPeerDisconnected func(string)
	IPs                []string
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
	tun     *water.Interface
	mtu     int
	ids     chan string
}

type peerWithIP struct {
	*wrtcconn.Peer
	ip  net.IP
	net *net.IPNet
}

func NewAdapter(
	signaler string,
	key string,
	ice []string,
	config *AdapterConfig,
	ctx context.Context,
) *Adapter {
	ictx, cancel := context.WithCancel(ctx)

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
	a.tun, err = water.New(water.Config{
		DeviceType:             water.TUN,
		PlatformSpecificParams: getPlatformSpecificParams(a.config.Device),
	})
	if err != nil {
		return err
	}

	for _, rawIP := range a.config.IPs {
		ip, _, err := net.ParseCIDR(rawIP)
		if err != nil {
			if a.config.Verbose {
				log.Println("Could not parse IP address, skipping")
			}

			continue
		}

		if err = setIPAddress(a.tun.Name(), rawIP, ip.To4() != nil); err != nil {
			return err
		}
	}

	data, err := json.Marshal(a.config.IPs)
	if err != nil {
		return err
	}
	a.config.AdapterConfig.ID = string(data)

	a.adapter = wrtcconn.NewAdapter(
		a.signaler,
		a.key,
		strings.Split(strings.Join(a.ice, ","), ","),
		[]string{"primary"},
		a.config.AdapterConfig,
		a.ctx,
	)

	a.ids, err = a.adapter.Open()
	if err != nil {
		return err
	}

	a.mtu, err = getMTU(a.tun.Name())

	return err
}

func (a *Adapter) Close() error {
	if err := a.tun.Close(); err != nil {
		return err
	}

	return a.adapter.Close()
}

func (a *Adapter) Wait() error {
	peers := map[string]*peerWithIP{}
	var peersLock sync.Mutex

	go func() {
		sem := semaphore.NewWeighted(int64(a.config.Parallel))

		for {
			buf := make([]byte, a.mtu+headerLength)

			if _, err := a.tun.Read(buf); err != nil {
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

				var dst net.IP
				var packet layers.IPv4
				if err := packet.DecodeFromBytes(buf, gopacket.NilDecodeFeedback); err != nil {
					var packet layers.IPv6
					if err := packet.DecodeFromBytes(buf, gopacket.NilDecodeFeedback); err != nil {
						if a.config.Verbose {
							log.Println("Could not unmarshal packet, skipping")
						}

						return
					} else {
						dst = packet.DstIP
					}
				} else {
					dst = packet.DstIP
				}

				peersLock.Lock()
				for _, peer := range peers {
					// Send if matching destination, multicast or broadcast IP
					if dst.Equal(peer.ip) || ((dst.IsMulticast() || dst.IsInterfaceLocalMulticast() || dst.IsInterfaceLocalMulticast()) && len(dst) == len(peer.ip)) || (peer.ip.To4() != nil && dst.Equal(getBroadcastAddr(peer.net))) {
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
			return nil
		case id := <-a.ids:
			if a.config.OnSignalerConnect != nil {
				a.config.OnSignalerConnect(id)
			}

			if err := setLinkUp(a.tun.Name()); err != nil {
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

				ips := []string{}
				if err := json.Unmarshal([]byte(peer.PeerID), &ips); err != nil {
					return
				}

				valid := false
				peersLock.Lock()
				for _, rawIP := range ips {
					ip, net, err := net.ParseCIDR(rawIP)
					if err != nil {
						if a.config.Verbose {
							log.Println("Could not parse IP address, skipping")
						}

						continue
					}

					peers[ip.String()] = &peerWithIP{peer, ip, net}

					valid = true
				}
				peersLock.Unlock()

				if !valid {
					return
				}

				for {
					buf := make([]byte, a.mtu+headerLength)

					if _, err := peer.Conn.Read(buf); err != nil {
						if a.config.Verbose {
							log.Println("Could not read from peer, stopping")
						}

						return
					}

					if _, err := a.tun.Write(buf); err != nil {
						if a.config.Verbose {
							log.Println("Could not write to TUN device, skipping")
						}

						continue
					}
				}
			}()
		}
	}
}

// See https://go.dev/play/p/Igo6Ct3gx_
func getBroadcastAddr(n *net.IPNet) net.IP {
	ip := make(net.IP, len(n.IP.To4()))

	binary.BigEndian.PutUint32(ip, binary.BigEndian.Uint32(n.IP.To4())|^binary.BigEndian.Uint32(net.IP(n.Mask).To4()))

	return ip
}
