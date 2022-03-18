package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pojntfx/webrtcfd/internal/persisters"
	"github.com/pojntfx/webrtcfd/internal/persisters/memory"
	"github.com/pojntfx/webrtcfd/internal/persisters/psql"
	"github.com/pojntfx/webrtcfd/internal/persisters/sqlite"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

const (
	topicMessagesPrefix = "messages."
	topicKick           = "kick"
)

var (
	errMissingCommunity   = errors.New("missing community")
	errMissingPassword    = errors.New("missing password")
	errMissingAPIPassword = errors.New("missing API password")

	errUnknownDBType = errors.New("unknown DB type")

	upgrader = websocket.Upgrader{}
)

type Input struct {
	Raddr       string `json:"raddr"`
	MessageType int    `json:"messageType"`
	P           []byte `json:"p"`
}

type Kick struct {
	Community string `json:"community"`
}

type connection struct {
	conn   *websocket.Conn
	closer chan struct{}
}

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	dbPath := filepath.Join(home, ".local", "share", "webrtcfd", "var", "lib", "webrtcfd", "communities.sqlite")

	laddr := flag.String("laddr", ":1337", "Listening address (can also be set using the PORT env variable)")
	heartbeat := flag.Duration("heartbeat", time.Second*10, "Time to wait for heartbeats")
	dbURL := flag.String("db-url", dbPath, "URL of database to use (i.e. postgres://myuser:mypassword@myhost:myport/mydatabase for PostgreSQL or mydatabase.sqlite for SQLite) (can also be set using the DATABASE_URL env variable)")
	dbType := flag.String("db-type", persisters.DBTypeSQLite, "Type of database to use (available are sqlite, psql and memory)")
	brokerURL := flag.String("broker-url", "redis://localhost:6379/1", "URL of broker to use (i.e. redis://myuser:mypassword@localhost:6379/1 for Redis (can also be set using the REDIS_URL env variable)")
	cleanup := flag.Bool("cleanup", false, "(Warning: Only enable this after stopping all other servers accessing the database!) Remove all ephermal communities from database and reset client counts before starting")
	apiPassword := flag.String("api-password", "", "Password for the management API (can also be set using the API_PASSWORD env variable)")
	ephermalCommunities := flag.Bool("ephermal-communities", true, "Enable the creation of ephermal communities")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")

	flag.Parse()

	if *verbose {
		boil.DebugMode = true
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	if p := os.Getenv("API_PASSWORD"); p != "" {
		if *verbose {
			log.Println("Using password from API_PASSWORD env variable")
		}

		*apiPassword = p
	}

	if u := os.Getenv("DATABASE_URL"); u != "" {
		if *verbose {
			log.Println("Using database URL from DATABASE_URL env variable")
		}

		*dbURL = u
	}

	if u := os.Getenv("REDIS_URL"); u != "" {
		if *verbose {
			log.Println("Using broker URL from REDIS_URL env variable")
		}

		*brokerURL = u
	}

	if strings.TrimSpace(*apiPassword) == "" {
		panic(errMissingAPIPassword)
	}

	var communities persisters.CommunitiesPersister
	switch *dbType {
	case persisters.DBTypeSQLite:
		communities = sqlite.NewCommunitiesPersister()
	case persisters.DBTypePSQL:
		communities = psql.NewCommunitiesPersister()
	case persisters.DBTypeMemory:
		communities = memory.NewCommunitiesPersister()
	default:
		panic(errUnknownDBType)
	}

	if err := communities.Open(*dbURL); err != nil {
		panic(err)
	}

	if *cleanup {
		if err := communities.Cleanup(ctx); err != nil {
			panic(err)
		}
	}

	srv := &http.Server{Addr: addr.String()}

	var connectionsLock sync.Mutex
	connections := map[string]map[string]connection{}

	redisURL, err := redis.ParseURL(*brokerURL)
	if err != nil {
		panic(err)
	}

	broker := redis.NewClient(redisURL).WithContext(ctx)

	s := make(chan os.Signal)
	signal.Notify(s, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-s

		log.Println("Gracefully shutting down signaling server")

		s := make(chan os.Signal)
		signal.Notify(s, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-s

			log.Println("Forcing shutdown of signaling server")

			cancel()

			if err := broker.Close(); err != nil {
				if err != context.Canceled && err != redis.ErrClosed {
					panic(err)
				}
			}

			if err := srv.Shutdown(ctx); err != nil {
				if err != context.Canceled {
					panic(err)
				}
			}
		}()

		connectionsLock.Lock()
		for c := range connections {
			for range connections[c] {
				if err := communities.RemoveClientFromCommunity(ctx, c); err != nil {
					panic(err)
				}
			}
		}
		connectionsLock.Unlock()

		cancel()

		if err := broker.Close(); err != nil {
			if err != context.Canceled && err != redis.ErrClosed {
				panic(err)
			}
		}

		if err := srv.Shutdown(ctx); err != nil {
			if err != context.Canceled {
				panic(err)
			}
		}
	}()

	kickPubsub := broker.Subscribe(ctx, topicKick)
	defer func() {
		if err := kickPubsub.Close(); err != nil {
			panic(err)
		}
	}()

	kicks := kickPubsub.Channel()

	srv.Handler = http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		raddr := uuid.New().String()

		defer func() {
			err := recover()
			switch err {
			case nil:
				log.Println("closed connection for client with address", raddr)
			case http.StatusUnauthorized:
				fallthrough
			case http.StatusNotImplemented:
				log.Println("closed connection for client with address", raddr+":", err)
			case http.StatusNotFound:
				log.Println("closed connection for client with address", raddr+":", err)
			default:
				rw.WriteHeader(http.StatusInternalServerError)

				log.Println("closed connection for client with address", raddr+":", err)
			}
		}()

		switch r.Method {
		case http.MethodGet:
			community := r.URL.Query().Get("community")
			if strings.TrimSpace(community) == "" {
				// List communities
				_, p, ok := r.BasicAuth()
				if !ok || p != *apiPassword {
					rw.WriteHeader(http.StatusUnauthorized)

					panic(http.StatusUnauthorized)
				}

				pc, err := communities.GetCommunities(ctx)
				if err != nil {
					panic(err)
				}

				j, err := json.Marshal(pc)
				if err != nil {
					panic(err)
				}

				if _, err := fmt.Fprint(rw, string(j)); err != nil {
					panic(err)
				}

				return
			}

			// Create ephermal community
			password := r.URL.Query().Get("password")
			if strings.TrimSpace(password) == "" {
				panic(errMissingPassword)
			}

			if err := communities.AddClientsToCommunity(ctx, community, password, *ephermalCommunities); err != nil {
				if err == persisters.ErrWrongPassword || err == persisters.ErrEphermalCommunitiesDisabled {
					rw.WriteHeader(http.StatusUnauthorized)

					panic(http.StatusUnauthorized)
				} else {
					panic(err)
				}
			}

			defer func() {
				if err := communities.RemoveClientFromCommunity(ctx, community); err != nil {
					panic(err)
				}
			}()

			conn, err := upgrader.Upgrade(rw, r, nil)
			if err != nil {
				panic(err)
			}

			defer func() {
				connectionsLock.Lock()
				delete(connections[community], raddr)
				if len(connections[community]) <= 0 {
					delete(connections, community)
				}
				connectionsLock.Unlock()

				log.Println("Disconnected from client with address", raddr, "in community", community)

				if err := conn.Close(); err != nil {
					panic(err)
				}
			}()

			connectionsLock.Lock()
			if _, exists := connections[community]; !exists {
				connections[community] = map[string]connection{}
			}
			connections[community][raddr] = connection{
				conn:   conn,
				closer: make(chan struct{}),
			}
			connectionsLock.Unlock()

			log.Println("Connected to client with address", raddr, "in community", community)

			if err := conn.SetReadDeadline(time.Now().Add(*heartbeat)); err != nil {
				panic(err)
			}
			conn.SetPongHandler(func(string) error {
				return conn.SetReadDeadline(time.Now().Add(*heartbeat))
			})

			pings := time.NewTicker(*heartbeat / 2)
			defer pings.Stop()

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
						log.Println("Received message with type", messageType, "from client with address", raddr, "in community", community)
					}

					data, err := json.Marshal(Input{raddr, messageType, p})
					if err != nil {
						errs <- err

						return
					}

					if err := broker.Publish(ctx, topicMessagesPrefix+community, data); err.Err() != nil {
						errs <- err.Err()

						return
					}
				}
			}()

			pubsub := broker.Subscribe(ctx, topicMessagesPrefix+community)
			defer func() {
				if err := pubsub.Close(); err != nil {
					panic(err)
				}
			}()

			inputs := pubsub.Channel()

			for {
				select {
				case <-connections[community][raddr].closer:
					return
				case err := <-errs:
					panic(err)
				case rawInput := <-inputs:
					var input Input
					if err := json.Unmarshal([]byte(rawInput.Payload), &input); err != nil {
						panic(err)
					}

					// Prevent sending message back to sender
					if input.Raddr == raddr {
						continue
					}

					if err := conn.WriteMessage(input.MessageType, input.P); err != nil {
						panic(err)
					}

					if err := conn.SetWriteDeadline(time.Now().Add(*heartbeat)); err != nil {
						panic(err)
					}
				case <-pings.C:
					if *verbose {
						log.Println("Sending ping to client with address", raddr, "in community", community)
					}

					if err := conn.SetWriteDeadline(time.Now().Add(*heartbeat)); err != nil {
						panic(err)
					}

					if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
						panic(err)
					}
				}
			}
		case http.MethodPost:
			// Create persistent community
			_, p, ok := r.BasicAuth()
			if !ok || p != *apiPassword {
				rw.WriteHeader(http.StatusUnauthorized)

				panic(http.StatusUnauthorized)
			}

			password := r.URL.Query().Get("password")
			if strings.TrimSpace(password) == "" {
				panic(errMissingPassword)
			}

			community := r.URL.Query().Get("community")
			if strings.TrimSpace(community) == "" {
				panic(errMissingCommunity)
			}

			c, err := communities.CreatePersistentCommunity(ctx, community, password)
			if err != nil {
				panic(err)
			}

			cc := persisters.Community{
				ID:         c.ID,
				Clients:    c.Clients,
				Persistent: c.Persistent,
			}

			j, err := json.Marshal(cc)
			if err != nil {
				panic(err)
			}

			if _, err := fmt.Fprint(rw, string(j)); err != nil {
				panic(err)
			}

			return
		case http.MethodDelete:
			// Delete persistent community
			_, p, ok := r.BasicAuth()
			if !ok || p != *apiPassword {
				rw.WriteHeader(http.StatusUnauthorized)

				panic(http.StatusUnauthorized)
			}

			community := r.URL.Query().Get("community")
			if strings.TrimSpace(community) == "" {
				panic(errMissingCommunity)
			}

			if err := communities.DeleteCommunity(ctx, community); err != nil {
				if err == sql.ErrNoRows {
					rw.WriteHeader(http.StatusNotFound)

					panic(http.StatusNotFound)
				} else {
					panic(err)
				}
			}

			data, err := json.Marshal(Kick{community})
			if err != nil {
				panic(err)
			}

			if err := broker.Publish(ctx, topicKick, data); err.Err() != nil {
				panic(err)
			}

			return
		default:
			rw.WriteHeader(http.StatusNotImplemented)

			panic(http.StatusNotImplemented)
		}
	})

	errs := make(chan error)
	go func() {
		for {
			rawKick := <-kicks

			if rawKick == nil {
				// Channel closed
				return
			}

			var kick Kick
			if err := json.Unmarshal([]byte(rawKick.Payload), &kick); err != nil {
				errs <- err

				return
			}

			c, ok := connections[kick.Community]
			if !ok {
				continue
			}

			for _, conn := range c {
				close(conn.closer)
			}
		}
	}()

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			if err == http.ErrServerClosed {
				return
			}

			errs <- err

			return
		}
	}()

	log.Println("Listening on address", addr.String())

	for err := range errs {
		if err != nil {
			panic(err)
		}
	}
}
