package wrtcip

import (
	"fmt"
	"net"
	"os/exec"

	"github.com/songgao/water"
)

func setupTUN(name string, ips []string) (*water.Interface, int, error) {
	tun, err := water.New(water.Config{
		DeviceType: water.TUN,
		PlatformSpecificParams: water.PlatformSpecificParams{
			ComponentID: "tap0901",
			Network:     ips[0],
		},
	})
	if err != nil {
		return nil, 0, err
	}

	ip, _, err := net.ParseCIDR(ips[0])
	if err != nil {
		return tun, 0, err
	}

	if ip.To4() != nil {
		output, err := exec.Command("netsh", "interface", "ipv4", "set", "address", tun.Name(), "static", ips[0]).CombinedOutput()
		if err != nil {
			return tun, 0, fmt.Errorf("could not add IPv4 address to interface: %v: %v", string(output), err)
		}
	} else {
		output, err := exec.Command("netsh", "interface", "ipv6", "set", "address", tun.Name(), ips[0]).CombinedOutput()
		if err != nil {
			return tun, 0, fmt.Errorf("could not add IPv6 address to interface: %v: %v", string(output), err)
		}
	}

	iface, err := net.InterfaceByName(tun.Name())
	if err != nil {
		return tun, 0, err
	}

	return tun, iface.MTU, nil
}
