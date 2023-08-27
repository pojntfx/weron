//go:build !(windows || linux)
// +build !windows,!linux

package wrtcip

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"

	"github.com/songgao/water"
)

func setupTUN(name string, ips []string) (*water.Interface, int, error) {
	tun, err := water.New(water.Config{
		DeviceType:             water.TUN,
		PlatformSpecificParams: water.PlatformSpecificParams{},
	})
	if err != nil {
		return nil, 0, err
	}

	for _, rawIP := range ips {
		ip, _, err := net.ParseCIDR(rawIP)
		if err != nil {
			return tun, 0, err
		}

		if ip.To4() != nil {
			// macOS does not support IPv4 TUN
			if runtime.GOOS == "darwin" && ip.To4() != nil {
				continue
			}

			output, err := exec.Command("ifconfig", tun.Name(), "inet", rawIP).CombinedOutput()
			if err != nil {
				return tun, 0, fmt.Errorf("could not add IPv4 address to interface: %v: %v", string(output), err)
			}
		} else {
			output, err := exec.Command("ifconfig", tun.Name(), "inet6", "add", rawIP).CombinedOutput()
			if err != nil {
				return tun, 0, fmt.Errorf("could not add IPv6 address to interface: %v: %v", string(output), err)
			}
		}
	}

	iface, err := net.InterfaceByName(tun.Name())
	if err != nil {
		return tun, 0, err
	}

	return tun, iface.MTU, nil
}
