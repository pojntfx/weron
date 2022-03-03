package main

import (
	"errors"
	"flag"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/uuid"
	"github.com/gorilla/websocket"
)

var (
	errMissingPath        = errors.New("missing path")
	errMissingCommunityID = errors.New("missing community ID")

	upgrader = websocket.Upgrader{}
)

func main() {
	laddr := flag.String("laddr", ":1337", "Listening address")
	heartbeat := flag.Duration("heartbeat", time.Second*10, "Time to wait for heartbeats")

	flag.Parse()

	log.Println("Listening on", *laddr)

	// TODO: Nest connections in `community` struct and only broadcast within community
	var connectionsLock sync.Mutex
	connections := map[string]*websocket.Conn{}

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
				if len(path) < 1 {
					panic(errMissingPath)
				}

				communityID := path[len(path)-1]
				if strings.TrimSpace(communityID) == "" {
					panic(errMissingCommunityID)
				}

				conn, err := upgrader.Upgrade(rw, r, nil)
				if err != nil {
					panic(err)
				}

				ownID, err := uuid.NewV4()
				if err != nil {
					panic(err)
				}

				connectionsLock.Lock()
				connections[ownID.String()] = conn
				connectionsLock.Unlock()

				defer func() {
					connectionsLock.Lock()
					delete(connections, ownID.String())
					connectionsLock.Unlock()

					log.Println("Disconnected from client with address", r.RemoteAddr, "in community", communityID)

					if err := conn.Close(); err != nil {
						panic(err)
					}
				}()

				log.Println("Connected to client with address", r.RemoteAddr, "in community", communityID)

				if err := conn.SetReadDeadline(time.Now().Add(*heartbeat)); err != nil {
					panic(err)
				}
				conn.SetPongHandler(func(string) error {
					return conn.SetReadDeadline(time.Now().Add(*heartbeat))
				})

				pings := time.NewTicker(*heartbeat / 2)
				defer pings.Stop()

				inputs := make(chan []byte)
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

						inputs <- msg
					}
				}()

				for {
					select {
					case err := <-errs:
						panic(err)
					case input := <-inputs:
						for id, conn := range connections {
							if id == ownID.String() {
								continue
							}

							if err := conn.WriteJSON(input); err != nil {
								panic(err)
							}

							if err := conn.SetWriteDeadline(time.Now().Add(*heartbeat)); err != nil {
								panic(err)
							}
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
