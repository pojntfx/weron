package wrtceth

import (
	"net"

	"github.com/songgao/water"
)

func getPlatformSpecificParams(name string) water.PlatformSpecificParams {
	return water.PlatformSpecificParams{
		InterfaceName: name,
	}
}

func setMACAddress(linkName string, hwaddr string) (string, error) {
	return hwaddr, nil
}

func getMTU(linkName string) (int, error) {
	iface, err := net.InterfaceByName(linkName)
	if err != nil {
		return -1, err
	}

	return iface.MTU, nil
}

func setLinkUp(linkName string) error {
	return nil
}
