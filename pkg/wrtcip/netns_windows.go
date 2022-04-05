package wrtcip

import (
	"fmt"
	"net"

	"github.com/songgao/water"
	"os/exec"
)

func getPlatformSpecificParams(name string) water.PlatformSpecificParams {
	return water.PlatformSpecificParams{}
}

func setIPAddress(linkName string, ipaddr string, ipv4 bool) error {
	if ipv4 {
		output, err := exec.Command("netsh", "interface", "ipv4", "set", "address", linkName, "static", ipaddr).CombinedOutput()
		if err != nil {
			return fmt.Errorf("could not add IPv4 address to interface: %v: %v", string(output), err)
		}
	} else {
		output, err := exec.Command("netsh", "interface", "ipv6", "set", "address", linkName, ipaddr).CombinedOutput()
		if err != nil {
			return fmt.Errorf("could not add IPv6 address to interface: %v: %v", string(output), err)
		}
	}

	return nil
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
