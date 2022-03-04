package main

import (
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var (
	errMissingPath        = errors.New("missing path")
	errMissingCommunityID = errors.New("missing community ID")

	upgrader = websocket.Upgrader{}
)

type input struct {
	messageType int
	p           []byte
}

func main() {
	laddr := flag.String("laddr", ":1337", "Listening address")
	heartbeat := flag.Duration("heartbeat", time.Second*10, "Time to wait for heartbeats")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")

	flag.Parse()

	addr, err := net.ResolveTCPAddr("tcp", *laddr)
	if err != nil {
		panic(err)
	}

	if port := os.Getenv("PORT"); port != "" {
		if *verbose {
			log.Println("Using port from PORT env variable")
		}

		p, err := strconv.Atoi(port)
		if err != nil {
			panic(err)
		}

		addr.Port = p
	}

	log.Println("Listening on address", addr.String())

	var communitiesLock sync.Mutex
	communities := map[string]map[string]*websocket.Conn{}

	panic(
		http.ListenAndServe(
			addr.String(),
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

				communitiesLock.Lock()
				if _, exists := communities[communityID]; !exists {
					communities[communityID] = map[string]*websocket.Conn{}
				}
				communities[communityID][r.RemoteAddr] = conn
				communitiesLock.Unlock()

				defer func() {
					communitiesLock.Lock()
					delete(communities[communityID], r.RemoteAddr)
					if len(communities[communityID]) <= 0 {
						delete(communities, communityID)
					}
					communitiesLock.Unlock()

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

				inputs := make(chan input)
				errs := make(chan error)
				go func() {
					for {
						messageType, p, err := conn.ReadMessage()
						if err != nil {
							if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNoStatusReceived) {
								errs <- err
							}

							errs <- nil

							return
						}

						if *verbose {
							log.Println("Received message with type", messageType, "from client with address", r.RemoteAddr, "in community", communityID)
						}

						inputs <- input{messageType, p}
					}
				}()

				for {
					select {
					case err := <-errs:
						panic(err)
					case input := <-inputs:
						for id, conn := range communities[communityID] {
							if id == r.RemoteAddr {
								continue
							}

							if *verbose {
								log.Println("Sending message with type", input.messageType, "to client with address", r.RemoteAddr, "in community", communityID)
							}

							if err := conn.WriteMessage(input.messageType, input.p); err != nil {
								panic(err)
							}

							if err := conn.SetWriteDeadline(time.Now().Add(*heartbeat)); err != nil {
								panic(err)
							}
						}
					case <-pings.C:
						if *verbose {
							log.Println("Sending ping to client with address", r.RemoteAddr, "in community", communityID)
						}

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
