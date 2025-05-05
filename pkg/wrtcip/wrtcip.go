package wrtcip

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"runtime"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"

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

	ErrMissingIPs = errors.New("no IPs provided")
)

// AdapterConfig configures the adapter
type AdapterConfig struct {
	*wrtcconn.NamedAdapterConfig
	Device             string       // Name to give to the TUN device
	OnSignalerConnect  func(string) // Handler to be called when the adapter has connected to the signaler
	OnPeerConnect      func(string) // Handler to be called when the adapter has connected to a peer
	OnPeerDisconnected func(string) // Handler to be called when the adapter has received a message
	CIDRs              []string     // IPv4 & IPv6 networks to join
	MaxRetries         int          // Maximum amount of IP address to try and claim before giving up
	Parallel           int          // Maximum amount of goroutines to use to unmarshal IP packets
	Static             bool         // Claim the exact IP specified in the CIDR notation instead of selecting a random one from the networks
}

// Adapter provides an IP service
type Adapter struct {
	signaler string
	key      string
	ice      []string
	config   *AdapterConfig
	ctx      context.Context

	cancel  context.CancelFunc
	adapter *wrtcconn.NamedAdapter
	tun     *water.Interface
	ids     chan string

	mtu     int
	mtuCond *sync.Cond
}

type peerWithIP struct {
	*wrtcconn.Peer
	ip  net.IP
	net *net.IPNet
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

		mtuCond: sync.NewCond(&sync.Mutex{}),
	}
}

// Open connects the adapter to the signaler
func (a *Adapter) Open() error {
	log.Trace().Msg("Opening adapter")

	for _, rawIP := range a.config.CIDRs {
		ip, _, err := net.ParseCIDR(rawIP)
		if err != nil {
			return err
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

	var err error
	a.ids, err = a.adapter.Open()
	if err != nil {
		return err
	}

	return err
}

// Close disconnects the adapter from the signaler and closes the TUN device
func (a *Adapter) Close() error {
	log.Trace().Msg("Closing adapter")

	if a.tun != nil {
		if err := a.tun.Close(); err != nil {
			return err
		}
	}

	return a.adapter.Close()
}

// Wait starts the transmission loop
func (a *Adapter) Wait() error {
	peers := map[string]*peerWithIP{}
	var peersLock sync.Mutex

	for {
		select {
		case <-a.ctx.Done():
			log.Trace().Err(a.ctx.Err()).Msg("Context cancelled")

			if err := a.ctx.Err(); err != context.Canceled {
				return err
			}

			return nil
		case err := <-a.adapter.Err():
			return err
		case id := <-a.ids:
			log.Debug().Str("id", id).Msg("Connected to signaler")

			if a.config.OnSignalerConnect != nil {
				a.config.OnSignalerConnect(id)
			}

			ips := []string{}
			if err := json.Unmarshal([]byte(id), &ips); err != nil {
				return err
			}

			if len(ips) <= 0 {
				return ErrMissingIPs
			}

			// Close old TUN device if it isn't already closed
			if a.tun != nil {
				_ = a.tun.Close()
			}

			a.mtuCond.L.Lock()
			var err error
			a.tun, a.mtu, err = setupTUN(a.config.Device, ips)
			if err != nil {
				a.mtuCond.L.Unlock()

				return err
			}
			// Signal that the MTU is available/the TUN device is started
			a.mtuCond.Broadcast()
			a.mtuCond.L.Unlock()

			go func() {
				sem := semaphore.NewWeighted(int64(a.config.Parallel))

				for {
					buf := make([]byte, a.mtu+headerLength) // No need for the MTU cond here since its guaranteed to be set

					if _, err := a.tun.Read(buf); err != nil {
						log.Debug().Err(err).Msg("Could not read from TUN device, returning")

						return
					}

					go func() {
						if err := sem.Acquire(a.ctx, 1); err != nil {
							log.Debug().Err(err).Msg("Could not acquire semaphore, stopping")

							return
						}
						defer sem.Release(1)

						var dst net.IP
						var packet layers.IPv4
						if err := packet.DecodeFromBytes(buf, gopacket.NilDecodeFeedback); err != nil {
							var packet layers.IPv6
							if err := packet.DecodeFromBytes(buf, gopacket.NilDecodeFeedback); err != nil {
								log.Debug().Err(err).Msg("Could not unmarshal packet, stopping")

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
		case peer := <-a.adapter.Accept():
			log.Debug().Str("channelID", peer.ChannelID).Str("peerID", peer.PeerID).Msg("Connected to peer")

			go func() {
				if a.config.OnPeerConnect != nil {
					a.config.OnPeerConnect(peer.PeerID)
				}

				ips := []string{}
				if err := json.Unmarshal([]byte(peer.PeerID), &ips); err != nil {
					log.Debug().
						Str("channelID", peer.ChannelID).
						Str("peerID", peer.PeerID).
						Err(err).Msg("Could not parse local IP address, stopping")

					return
				}

				valid := false
				peersLock.Lock()
				for _, rawIP := range ips {
					ip, net, err := net.ParseCIDR(rawIP)
					if err != nil {
						log.Debug().
							Str("channelID", peer.ChannelID).
							Str("peerID", peer.PeerID).
							Err(err).
							Msg("Could not parse local IP address, continuing")

						continue
					}

					peers[ip.String()] = &peerWithIP{peer, ip, net}

					valid = true
				}
				peersLock.Unlock()

				defer func() {
					log.Debug().Str("channelID", peer.ChannelID).Str("peerID", peer.PeerID).Msg("Disconnected from peer")

					if a.config.OnPeerDisconnected != nil {
						a.config.OnPeerDisconnected(peer.PeerID)
					}

					peersLock.Lock()
					for _, rawIP := range ips {
						ip, _, err := net.ParseCIDR(rawIP)
						if err != nil {
							continue
						}

						delete(peers, ip.String())
					}
					peersLock.Unlock()
				}()

				if !valid {
					log.Debug().
						Str("channelID", peer.ChannelID).
						Str("peerID", peer.PeerID).
						Msg("Got peer with invalid IP addresses, stopping")

					return
				}

				for {
					a.mtuCond.L.Lock()
					if a.mtu <= 0 {
						a.mtuCond.Wait()
					}
					buf := make([]byte, a.mtu+headerLength)
					a.mtuCond.L.Unlock()

					if _, err := peer.Conn.Read(buf); err != nil {
						log.Debug().
							Err(err).
							Str("channelID", peer.ChannelID).
							Str("peerID", peer.PeerID).
							Msg("Could not read from peer, stopping")

						return
					}

					if _, err := a.tun.Write(buf); err != nil {
						log.Debug().
							Err(err).
							Str("channelID", peer.ChannelID).
							Str("peerID", peer.PeerID).
							Msg("Could not write to TUN device, continuing")

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
