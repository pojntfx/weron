package main

import (
	"context"
	"log"

	"github.com/pojntfx/webrtcfd/pkg/overlay"
)

func init() {
	community := overlay.NewCommunity(
		"wss://mypeerid:mypassword@signaler.pojtinger.com:1337#mycommunity",
		[]string{"stun:stun.l.google.com:19302", "myusername:mycredential@turn:turn.l.google.com:69420"},
	)

	if err := community.Join(context.Background()); err != nil {
		return err
	}
	defer community.Leave()

	for {
		peerID, conn, err := community.Accept()
		if err != nil {
			return err
		}

		go func() {
			log.Println("Peer connected", peerID)

			for {
				buf := make([]byte, 1024)
				if _, err := conn.Read(buf); err != nil {
					break
				}

				log.Println("Received message from peer", peerID)

				if _, err := conn.Write([]byte("Hello from mypeer!")); err != nil {
					break
				}
			}

			log.Println("Peer disconnected", peerID)
		}()
	}

}
