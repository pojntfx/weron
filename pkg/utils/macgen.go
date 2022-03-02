package utils

import (
	"crypto/rand"
	"fmt"
)

func GenerateMACAddress() (string, error) {
	// See https://stackoverflow.com/questions/21018729/generate-mac-address-in-go
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	buf[0] |= 2 // Set local bit

	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", buf[0], buf[1], buf[2], buf[3], buf[4], buf[5]), nil
}
