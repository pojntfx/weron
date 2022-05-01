package wrtcip

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/netip"
	"runtime"
	"strings"
	"sync"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	jsoniter "github.com/json-iterator/go"
	"github.com/pojntfx/weron/pkg/services"
	"github.com/pojntfx/weron/pkg/wrtcconn"
	"github.com/songgao/water"
	"golang.org/x/sync/semaphore"
)

const (
	headerLength = 22
)

var (
	json = jsoniter.ConfigCompatibleWithStandardLibrary
)

type AdapterConfig struct {
	*wrtcconn.NamedAdapterConfig
	Device             string
	OnSignalerConnect  func(string)
	OnPeerConnect      func(string)
	OnPeerDisconnected func(string)
	CIDRs              []string
	MaxRetries         int
	Parallel           int
	Static             bool
}

type Adapter struct {
	signaler string
	key      string
	ice      []string
	config   *AdapterConfig
	ctx      context.Context

	cancel  context.CancelFunc
	adapter *wrtcconn.NamedAdapter
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
	a.tun, err = water.New(water.Config{
		DeviceType:             water.TUN,
		PlatformSpecificParams: getPlatformSpecificParams(a.config.Device),
	})
	if err != nil {
		return err
	}

	for _, rawIP := range a.config.CIDRs {
		ip, _, err := net.ParseCIDR(rawIP)
		if err != nil {
			if a.config.Verbose {
				log.Println("Could not parse IP address, skipping")
			}

			continue
		}

		// macOS does not support IPv4 TUN
		if runtime.GOOS == "darwin" && ip.To4() != nil {
			continue
		}

	}

	names := []string{}
	if a.config.Static {
		for _, cidr := range a.config.CIDRs {
			if _, err := netip.ParsePrefix(cidr); err != nil {
				return err
			}
		}

		name, err := json.Marshal(a.config.CIDRs)
		if err != nil {
			return err
		}

		names = append(names, string(name))
	} else {
		rawNames := make([][]string, a.config.MaxRetries)
		for _, cidr := range a.config.CIDRs {
			prefix, err := netip.ParsePrefix(cidr)
			if err != nil {
				return err
			}

			cidrIPs := []string{}
			i := 0
			for addr := prefix.Addr(); prefix.Contains(addr); addr = addr.Next() {
				if i >= a.config.MaxRetries+2 {
					break
				}

				cidrIPs = append(cidrIPs, fmt.Sprintf("%v/%v", addr.String(), prefix.Bits()))

				i++
			}

			if prefix.Addr().Is4() && len(cidrIPs) > 2 {
				cidrIPs = cidrIPs[1 : len(cidrIPs)-1]
			}

			for i, cidrIP := range cidrIPs {
				if i >= a.config.MaxRetries {
					break
				}

				rawNames[i] = append(rawNames[i], cidrIP)
			}
		}

		for _, rawName := range rawNames {
			name, err := json.Marshal(rawName)
			if err != nil {
				return err
			}

			names = append(names, string(name))
		}
	}

	a.config.NamedAdapterConfig.Names = names
	a.config.NamedAdapterConfig.IsIDClaimed = func(theirRawIPs map[string]struct{}, s string) bool {
		ourIPs := []string{}
		if err := json.Unmarshal([]byte(s), &ourIPs); err != nil {
			return true
		}

		for theirRawIP := range theirRawIPs {
			for _, ourRawIP := range ourIPs {
				theirIP, _, err := net.ParseCIDR(theirRawIP)
				if err != nil {
					return true
				}

				ourIP, _, err := net.ParseCIDR(ourRawIP)
				if err != nil {
					return true
				}

				if theirIP.Equal(ourIP) {
					return true
				}
			}
		}

		return false
	}

	a.adapter = wrtcconn.NewNamedAdapter(
		a.signaler,
		a.key,
		strings.Split(strings.Join(a.ice, ","), ","),
		[]string{services.IPPrimary},
		a.config.NamedAdapterConfig,
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
			return a.ctx.Err()
		case err := <-a.adapter.Err():
			return err
		case id := <-a.ids:
			if a.config.OnSignalerConnect != nil {
				a.config.OnSignalerConnect(id)
			}

			ips := []string{}
			if err := json.Unmarshal([]byte(id), &ips); err != nil {
				return err
			}

			for _, rawIP := range ips {
				ip, _, err := net.ParseCIDR(rawIP)
				if err != nil {
					if a.config.Verbose {
						log.Println("Could not parse IP address, skipping")
					}

					continue
				}

				// macOS does not support IPv4 TUN
				if runtime.GOOS == "darwin" && ip.To4() != nil {
					continue
				}

				if err = setIPAddress(a.tun.Name(), rawIP, ip.To4() != nil); err != nil {
					return err
				}
			}

			if err := setLinkUp(a.tun.Name()); err != nil {
				return err
			}
		case peer := <-a.adapter.Accept():
			go func() {
				if a.config.OnPeerConnect != nil {
					a.config.OnPeerConnect(peer.PeerID)
				}

				ips := []string{}
				if err := json.Unmarshal([]byte(peer.PeerID), &ips); err != nil {
					if a.config.Verbose {
						log.Println("Could not parse local IP addresses, skipping")
					}

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

				defer func() {
					if a.config.OnPeerDisconnected != nil {
						a.config.OnPeerDisconnected(peer.PeerID)
					}

					peersLock.Lock()
					for _, ip := range ips {
						delete(peers, ip)
					}
					peersLock.Unlock()
				}()

				if !valid {
					if a.config.Verbose {
						log.Println("Got peer with invalid IP addresses, skipping")
					}

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
