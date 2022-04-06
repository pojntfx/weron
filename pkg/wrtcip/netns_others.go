//go:build !(windows || linux)
// +build !windows,!linux

package wrtcip

import (
	"fmt"
	"net"
	"os/exec"

	"github.com/songgao/water"
)

func getPlatformSpecificParams(name string) water.PlatformSpecificParams {
	return water.PlatformSpecificParams{}
}

func setIPAddress(linkName string, ipaddr string, ipv4 bool) error {
	if ipv4 {
		output, err := exec.Command("ifconfig", linkName, "inet", ipaddr).CombinedOutput()
		if err != nil {
			return fmt.Errorf("could not add IPv4 address to interface: %v: %v", string(output), err)
		}
	} else {
		output, err := exec.Command("ifconfig", linkName, "inet6", "add", ipaddr).CombinedOutput()
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
