package wrtcip

import (
	"net"

	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
)

func getPlatformSpecificParams(name string) water.PlatformSpecificParams {
	return water.PlatformSpecificParams{
		Name: name,
	}
}

func setIPAddress(linkName string, ipaddr string) error {
	link, err := netlink.LinkByName(linkName)
	if err != nil {
		return err
	}

	ip, err := netlink.ParseAddr(ipaddr)
	if err != nil {
		return err
	}

	return netlink.AddrAdd(link, ip)
}

func getMTU(linkName string) (int, error) {
	iface, err := net.InterfaceByName(linkName)
	if err != nil {
		return -1, err
	}

	return iface.MTU, nil
}

func setLinkUp(linkName string) error {
	link, err := netlink.LinkByName(linkName)
	if err != nil {
		return err
	}

	return netlink.LinkSetUp(link)
}
