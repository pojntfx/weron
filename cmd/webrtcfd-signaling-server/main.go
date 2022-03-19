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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	rediserr "github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pojntfx/webrtcfd/internal/brokers"
	"github.com/pojntfx/webrtcfd/internal/brokers/redis"
	"github.com/pojntfx/webrtcfd/internal/persisters"
	"github.com/pojntfx/webrtcfd/internal/persisters/memory"
	"github.com/pojntfx/webrtcfd/internal/persisters/psql"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

var (
	errMissingCommunity   = errors.New("missing community")
	errMissingPassword    = errors.New("missing password")
	errMissingAPIPassword = errors.New("missing API password")

	upgrader = websocket.Upgrader{}
)

type connection struct {
	conn   *websocket.Conn
	closer chan struct{}
}

func main() {
	laddr := flag.String("laddr", ":1337", "Listening address (can also be set using the PORT env variable)")
	heartbeat := flag.Duration("heartbeat", time.Second*10, "Time to wait for heartbeats")
	postgresURL := flag.String("postgres-url", "postgres://postgres@localhost:5432/webrtcfd_communities?sslmode=disable", "URL of PostgreSQL database to use (i.e. postgres://myuser:mypassword@myhost:myport/mydatabase) (can also be set using the DATABASE_URL env variable). If empty, a in-memory database will be used.")
	redisURL := flag.String("redis-url", "redis://localhost:6379/1", "URL of Redis database to use (i.e. redis://myuser:mypassword@localhost:6379/1) (can also be set using the REDIS_URL env variable). If empty, a in-process broker will be used.")
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

		*postgresURL = u
	}

	if u := os.Getenv("REDIS_URL"); u != "" {
		if *verbose {
			log.Println("Using broker URL from REDIS_URL env variable")
		}

		*redisURL = u
	}

	if strings.TrimSpace(*apiPassword) == "" {
		panic(errMissingAPIPassword)
	}

	var db persisters.CommunitiesPersister
	if strings.TrimSpace(*postgresURL) == "" {
		db = memory.NewCommunitiesPersister()
	} else {
		db = psql.NewCommunitiesPersister()
	}

	if err := db.Open(*postgresURL); err != nil {
		panic(err)
	}

	if *cleanup {
		if err := db.Cleanup(ctx); err != nil {
			panic(err)
		}
	}

	var broker brokers.CommunitiesBroker
	if strings.TrimSpace(*redisURL) == "" {
		// TODO: Add in-process broker
		panic("not implemented")
	} else {
		broker = redis.NewCommunitiesBroker()
	}

	if err := broker.Open(ctx, *redisURL); err != nil {
		panic(err)
	}

	srv := &http.Server{Addr: addr.String()}

	var connectionsLock sync.Mutex
	connections := map[string]map[string]connection{}

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
				if err != context.Canceled && err != rediserr.ErrClosed {
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
				if err := db.RemoveClientFromCommunity(ctx, c); err != nil {
					panic(err)
				}
			}
		}
		connectionsLock.Unlock()

		cancel()

		if err := broker.Close(); err != nil {
			if err != context.Canceled && err != rediserr.ErrClosed {
				panic(err)
			}
		}

		if err := srv.Shutdown(ctx); err != nil {
			if err != context.Canceled {
				panic(err)
			}
		}
	}()

	errs := make(chan error)
	kicks, closeKicks := broker.SubscribeToKicks(ctx, errs)
	defer func() {
		if err := closeKicks(); err != nil {
			panic(err)
		}
	}()

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

				pc, err := db.GetCommunities(ctx)
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

			if err := db.AddClientsToCommunity(ctx, community, password, *ephermalCommunities); err != nil {
				if err == persisters.ErrWrongPassword || err == persisters.ErrEphermalCommunitiesDisabled {
					rw.WriteHeader(http.StatusUnauthorized)

					panic(http.StatusUnauthorized)
				} else {
					panic(err)
				}
			}

			defer func() {
				if err := db.RemoveClientFromCommunity(ctx, community); err != nil {
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

					if err := broker.PublishInput(ctx, brokers.Input{
						Raddr:       raddr,
						MessageType: messageType,
						P:           p,
					}, community); err != nil {
						errs <- err

						return
					}
				}
			}()

			inputs, closeInputs := broker.SubscribeToInputs(ctx, errs, community)
			defer func() {
				if err := closeInputs(); err != nil {
					panic(err)
				}
			}()

			for {
				select {
				case <-connections[community][raddr].closer:
					return
				case err := <-errs:
					panic(err)
				case input := <-inputs:
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

			c, err := db.CreatePersistentCommunity(ctx, community, password)
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

			if err := db.DeleteCommunity(ctx, community); err != nil {
				if err == sql.ErrNoRows {
					rw.WriteHeader(http.StatusNotFound)

					panic(http.StatusNotFound)
				} else {
					panic(err)
				}
			}

			if err := broker.PublishKick(ctx, brokers.Kick{
				Community: community,
			}); err != nil {
				panic(err)
			}

			return
		default:
			rw.WriteHeader(http.StatusNotImplemented)

			panic(http.StatusNotImplemented)
		}
	})

	go func() {
		for {
			kick := <-kicks

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
				close(errs)

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
