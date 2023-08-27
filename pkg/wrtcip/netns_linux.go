package wrtcip

import (
	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
)

func setupTUN(name string, ips []string) (*water.Interface, int, error) {
	tun, err := water.New(water.Config{
		DeviceType: water.TUN,
		PlatformSpecificParams: water.PlatformSpecificParams{
			Name: name,
		},
	})
	if err != nil {
		return nil, 0, err
	}

	link, err := netlink.LinkByName(tun.Name())
	if err != nil {
		return tun, 0, err
	}

	for _, rawIP := range ips {
		ip, err := netlink.ParseAddr(rawIP)
		if err != nil {
			return tun, 0, err
		}

		if err := netlink.AddrAdd(link, ip); err != nil {
			return tun, 0, err
		}
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return tun, 0, err
	}

	return tun, link.Attrs().MTU, nil
}
