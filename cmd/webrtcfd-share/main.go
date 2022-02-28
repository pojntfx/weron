package main

import (
	"errors"
	"flag"
	"log"
	"strings"

	"github.com/pion/webrtc/v3"
)

var (
	errInvalidCommunity       = errors.New("invalid community")
	errInvalidPassword        = errors.New("invalid password")
	errInvalidTURNAddr        = errors.New("invalid TURN server address")
	errMissingTURNCredentials = errors.New("missing TURN server credentials")
)

func main() {
	path := flag.String("path", "file.webrtcfd", "Path to the file to share (will be created if it does not exist)")

	signaler := flag.String("signaler", "wss://weron.herokuapp.com/", "Address of the signaling server")
	stun := flag.String("stun", "stun:stun.l.google.com:19302", "Comma-seperated list of STUN servers in format stun:domain:port")
	turn := flag.String("turn", "", "Comma-seperated list of TURN servers in format username:credential@turn:domain:port")

	community := flag.String("community", "", "Community to join")
	password := flag.String("password", "", "Password for the community")

	flag.Parse()

	if strings.TrimSpace(*community) == "" {
		panic(errInvalidCommunity)
	}

	if strings.TrimSpace(*password) == "" {
		panic(errInvalidPassword)
	}

	ice := []webrtc.ICEServer{}

	for _, srv := range strings.Split(*stun, ",") {
		if srv == "" {
			continue
		}

		ice = append(ice, webrtc.ICEServer{
			URLs: []string{srv},
		})
	}

	for _, srv := range strings.Split(*turn, ",") {
		if srv == "" {
			continue
		}

		parts := strings.Split(srv, "@")
		if len(parts) < 2 {
			panic(errInvalidTURNAddr)
		}

		auth := strings.Split(parts[0], ":")
		if len(auth) < 2 {
			panic(errMissingTURNCredentials)
		}

		ice = append(ice, webrtc.ICEServer{
			URLs:           []string{srv},
			Username:       auth[0],
			Credential:     auth[1],
			CredentialType: webrtc.ICECredentialTypePassword,
		})
	}

	log.Println("Connecting to community", *community, "using signaler", *signaler, "to share file", *path)
}
