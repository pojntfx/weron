package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pojntfx/webrtcfd/internal/persisters"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

var (
	errMissingPath        = errors.New("missing path")
	errMissingCommunityID = errors.New("missing community ID")
	errMissingPassword    = errors.New("missing password")

	upgrader = websocket.Upgrader{}
)

type input struct {
	messageType int
	p           []byte
}

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	communitiesPath := filepath.Join(home, ".local", "share", "webrtcfd", "var", "lib", "webrtcfd", "communities.sqlite")

	laddr := flag.String("laddr", ":1337", "Listening address")
	heartbeat := flag.Duration("heartbeat", time.Second*10, "Time to wait for heartbeats")
	dbPath := flag.String("db", communitiesPath, "Database to use")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")

	flag.Parse()

	if *verbose {
		boil.DebugMode = true
	}

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

	communities := persisters.NewCommunitiesPersister(*dbPath)

	if err := communities.Open(); err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var connectionsLock sync.Mutex
	connections := map[string]map[string]*websocket.Conn{}

	panic(
		http.ListenAndServe(
			addr.String(),
			http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				raddr := base64.StdEncoding.EncodeToString(sha256.New().Sum([]byte(r.RemoteAddr))) // Obfuscate remote address to prevent processing GDPR-sensitive information

				defer func() {
					if err := recover(); err != nil {
						log.Println("closed connection for client with address", raddr+":", err)
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

				password := r.URL.Query().Get("password")
				if strings.TrimSpace(password) == "" {
					panic(errMissingPassword)
				}

				if err := communities.AddClientsToCommunity(ctx, communityID, password, false); err != nil {
					panic(err)
				}

				defer func() {
					if err := communities.RemoveClientFromCommunity(ctx, communityID); err != nil {
						panic(err)
					}
				}()

				conn, err := upgrader.Upgrade(rw, r, nil)
				if err != nil {
					panic(err)
				}

				defer func() {
					connectionsLock.Lock()
					delete(connections[communityID], raddr)
					if len(connections[communityID]) <= 0 {
						delete(connections, communityID)
					}
					connectionsLock.Unlock()

					log.Println("Disconnected from client with address", raddr, "in community", communityID)

					if err := conn.Close(); err != nil {
						panic(err)
					}
				}()

				connectionsLock.Lock()
				if _, exists := connections[communityID]; !exists {
					connections[communityID] = map[string]*websocket.Conn{}
				}
				connections[communityID][raddr] = conn
				connectionsLock.Unlock()

				log.Println("Connected to client with address", raddr, "in community", communityID)

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
							log.Println("Received message with type", messageType, "from client with address", raddr, "in community", communityID)
						}

						inputs <- input{messageType, p}
					}
				}()

				for {
					select {
					case err := <-errs:
						panic(err)
					case input := <-inputs:
						for id, conn := range connections[communityID] {
							if id == raddr {
								continue
							}

							if *verbose {
								log.Println("Sending message with type", input.messageType, "to client with address", raddr, "in community", communityID)
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
							log.Println("Sending ping to client with address", raddr, "in community", communityID)
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
