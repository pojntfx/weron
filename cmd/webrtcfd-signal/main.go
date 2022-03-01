package main

import (
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type inputMsg struct {
	Message string `json:"message"`
}

type outputMsg struct {
	Message     string `json:"message"`
	CommunityID string `json:"communityID"`
	PeerID      string `json:"peerID"`
}

var (
	errMissingPath        = errors.New("missing path")
	errMissingCommunityID = errors.New("missing community ID")
	errMissingPeerID      = errors.New("missing peer ID")

	upgrader = websocket.Upgrader{}
)

func main() {
	laddr := flag.String("laddr", ":1337", "Listening address")
	heartbeat := flag.Duration("heartbeat", time.Second*10, "Time to wait for heartbeats")

	flag.Parse()

	log.Println("Listening on", *laddr)

	panic(
		http.ListenAndServe(
			*laddr,
			http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				defer func() {
					if err := recover(); err != nil {
						log.Println("closed connection for client with address", r.RemoteAddr+":", err)
					}
				}()

				path := strings.Split(r.URL.Path, "/")
				if len(path) < 2 {
					panic(errMissingPath)
				}

				communityID := path[len(path)-2]
				if strings.TrimSpace(communityID) == "" {
					panic(errMissingCommunityID)
				}

				peerID := path[len(path)-1]
				if strings.TrimSpace(peerID) == "" {
					panic(errMissingPeerID)
				}

				conn, err := upgrader.Upgrade(rw, r, nil)
				if err != nil {
					panic(err)
				}
				defer func() {
					log.Println("Disconnected from client with address", r.RemoteAddr+", peer ID", peerID, "and community ID", communityID)

					if err := conn.Close(); err != nil {
						panic(err)
					}
				}()

				log.Println("Connected to client with address", r.RemoteAddr+", peer ID", peerID, "and community ID", communityID)

				if err := conn.SetReadDeadline(time.Now().Add(*heartbeat)); err != nil {
					panic(err)
				}
				conn.SetPongHandler(func(string) error {
					return conn.SetReadDeadline(time.Now().Add(*heartbeat))
				})

				pings := time.NewTicker(*heartbeat / 2)
				defer pings.Stop()

				inputs := make(chan inputMsg)
				errs := make(chan error)
				go func() {
					for {
						_, msg, err := conn.ReadMessage()
						if err != nil {
							if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNoStatusReceived) {
								errs <- err
							}

							errs <- nil

							return
						}

						var input inputMsg
						if err := json.Unmarshal(msg, &input); err != nil {
							errs <- err

							return
						}

						inputs <- input
					}
				}()

				for {
					select {
					case err := <-errs:
						panic(err)
					case input := <-inputs:
						if err := conn.WriteJSON(
							outputMsg{
								Message:     "You've sent: " + input.Message,
								CommunityID: communityID,
								PeerID:      peerID,
							},
						); err != nil {
							panic(err)
						}

						if err := conn.SetWriteDeadline(time.Now().Add(*heartbeat)); err != nil {
							panic(err)
						}
					case <-pings.C:
						if err := conn.SetWriteDeadline(time.Now().Add(*heartbeat)); err != nil {
							panic(err)
						}

						if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
							panic(err)
						}
					}
				}
			}),
		),
	)
}
