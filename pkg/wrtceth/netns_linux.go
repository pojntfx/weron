package wrtceth

import (
	"net"
	"strings"

	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
)

func getPlatformSpecificParams(name string) water.PlatformSpecificParams {
	return water.PlatformSpecificParams{
		Name: name,
	}
}

func setMACAddress(linkName string, hwaddr string) (string, error) {
	link, err := netlink.LinkByName(linkName)
	if err != nil {
		return "", err
	}

	var mac net.HardwareAddr
	if strings.TrimSpace(hwaddr) == "" {
		mac = link.Attrs().HardwareAddr
	} else {
		mac, err = net.ParseMAC(hwaddr)
		if err != nil {
			return "", err
		}
	}

	return mac.String(), netlink.LinkSetHardwareAddr(link, mac)
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
