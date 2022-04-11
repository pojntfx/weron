package wrtcip

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"log"
	"net"
	"runtime"
	"strings"
	"sync"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/mitchellh/mapstructure"
	v1 "github.com/pojntfx/webrtcfd/pkg/api/webrtc/v1"
	"github.com/pojntfx/webrtcfd/pkg/wrtcconn"
	"github.com/songgao/water"
	"golang.org/x/sync/semaphore"
)

const (
	headerLength = 22

	primaryChannelName = "wrtcip.primary"
	controlChannelName = "wrtcip.control"
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

		// macOS does not support IPv4 TUN
		if runtime.GOOS == "darwin" && ip.To4() != nil {
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
		[]string{primaryChannelName, controlChannelName},
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

	id := ""
	for {
		select {
		case <-a.ctx.Done():
			return nil
		case rid := <-a.ids:
			if a.config.OnSignalerConnect != nil {
				a.config.OnSignalerConnect(rid)
			}

			if err := setLinkUp(a.tun.Name()); err != nil {
				return err
			}

			id = rid
		case peer := <-a.adapter.Accept():
			switch peer.ChannelID {
			case primaryChannelName:
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
			case controlChannelName:
				go func() {
					defer func() {
						// Handle JSON parser errors when reading from connection
						if err := recover(); err != nil {
							if a.config.Verbose {
								log.Println("Could not read from peer, stopping")

								return
							}
						}
					}()

					for {
						d := json.NewDecoder(peer.Conn)

						var j interface{}
						if err := d.Decode(&j); err != nil {
							if err == io.EOF {
								if a.config.Verbose {
									log.Println("Could not read from peer, stopping")
								}

								return
							}

							if a.config.Verbose {
								log.Println("Could not read from peer, skipping")
							}

							continue
						}

						var message v1.Message
						if err := mapstructure.Decode(j, &message); err != nil {
							if a.config.Verbose {
								log.Println("Could not parse message from peer, skipping")
							}

							continue
						}

						switch message.Type {
						case v1.TypeApplication:
							var application v1.Application
							if err := mapstructure.Decode(j, &application); err != nil {
								if a.config.Verbose {
									log.Println("Could not parse message from peer, skipping")
								}

								continue
							}

							ips := []string{}
							if err := json.Unmarshal([]byte(id), &ips); err != nil {
								if a.config.Verbose {
									log.Println("Could not parse local IP addresses, skipping")
								}

								continue
							}

							duplicate := false
						l:
							for _, rawLocalIP := range ips {
								localIP, _, err := net.ParseCIDR(rawLocalIP)
								if err != nil {
									if a.config.Verbose {
										log.Println("Could not parse IP address, skipping")
									}

									break l
								}

								for _, rawRemoteIP := range application.IPs {
									remoteIP, _, err := net.ParseCIDR(rawRemoteIP)
									if err != nil {
										if a.config.Verbose {
											log.Println("Could not parse remote address, skipping")
										}

										break l
									}

									if localIP.Equal(remoteIP) {
										duplicate = true
									}
								}
							}

							if duplicate {
								d, err := json.Marshal(v1.NewRejection())
								if err != nil {
									if a.config.Verbose {
										log.Println("Could not marshal rejection, skipping")
									}

									continue
								}

								if _, err := peer.Conn.Write([]byte(string(d) + "\n")); err != nil {
									if a.config.Verbose {
										log.Println("Could not write to peer, skipping")
									}

									continue
								}
							}
						default:
							if a.config.Verbose {
								log.Println("Got unknown message type, skipping")
							}

							continue
						}
					}
				}()
			default:
				if a.config.Verbose {
					log.Println("Got unknown channel name, skipping")
				}

				continue
			}
		}
	}
}

// See https://go.dev/play/p/Igo6Ct3gx_
func getBroadcastAddr(n *net.IPNet) net.IP {
	ip := make(net.IP, len(n.IP.To4()))

	binary.BigEndian.PutUint32(ip, binary.BigEndian.Uint32(n.IP.To4())|^binary.BigEndian.Uint32(net.IP(n.Mask).To4()))

	return ip
}
