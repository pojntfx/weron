package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pojntfx/webrtcfd/pkg/wrtcsgl"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

var (
	errMissingCommunity = errors.New("missing community")
	errMissingPassword  = errors.New("missing password")

	upgrader = websocket.Upgrader{}
)

type connection struct {
	conn   *websocket.Conn
	closer chan struct{}
}

func main() {
	laddr := flag.String("laddr", ":1337", "Listening address (can also be set using the PORT env variable)")
	heartbeat := flag.Duration("heartbeat", time.Second*10, "Time to wait for heartbeats")
	postgresURL := flag.String("postgres-url", "", "URL of PostgreSQL database to use (i.e. postgres://myuser:mypassword@myhost:myport/mydatabase) (can also be set using the DATABASE_URL env variable). If empty, a in-memory database will be used.")
	redisURL := flag.String("redis-url", "", "URL of Redis database to use (i.e. redis://myuser:mypassword@localhost:6379/1) (can also be set using the REDIS_URL env variable). If empty, a in-process broker will be used.")
	cleanup := flag.Bool("cleanup", false, "(Warning: Only enable this after stopping all other servers accessing the database!) Remove all ephermal communities from database and reset client counts before starting")
	ephermalCommunities := flag.Bool("ephermal-communities", true, "Enable the creation of ephermal communities")
	apiUsername := flag.String("api-username", "admin", "Username for the management API (can also be set using the API_USERNAME env variable). Ignored if any of the OIDC parameters are set.")
	apiPassword := flag.String("api-password", "", "Password for the management API (can also be set using the API_PASSWORD env variable). Ignored if any of the OIDC parameters are set.")
	oidcIssuer := flag.String("oidc-issuer", "", "OIDC Issuer (i.e. https://pojntfx.eu.auth0.com/) (can also be set using the OIDC_ISSUER env variable)")
	oidcClientID := flag.String("oidc-client-id", "", "OIDC Client ID (i.e. myoidcclientid) (can also be set using the OIDC_CLIENT_ID env variable)")
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

	if u := os.Getenv("API_USERNAME"); u != "" {
		if *verbose {
			log.Println("Using username from API_USERNAME env variable")
		}

		*apiUsername = u
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

	if u := os.Getenv("OIDC_ISSUER"); u != "" {
		if *verbose {
			log.Println("Using OIDC issuer from OIDC_ISSUER env variable")
		}

		*oidcIssuer = u
	}

	if u := os.Getenv("OIDC_CLIENT_ID"); u != "" {
		if *verbose {
			log.Println("Using OIDC client ID from OIDC_CLIENT_ID env variable")
		}

		*oidcClientID = u
	}

	signaler := wrtcsgl.NewSignaler(
		addr.String(),
		*postgresURL,
		*redisURL,
		&wrtcsgl.SignalerConfig{
			Heartbeat:           *heartbeat,
			Cleanup:             *cleanup,
			EphermalCommunities: *ephermalCommunities,
			APIUsername:         *apiUsername,
			APIPassword:         *apiPassword,
			OIDCIssuer:          *oidcIssuer,
			OIDCClientID:        *oidcClientID,
			Verbose:             *verbose,
			OnConnect: func(raddr, community string) {
				log.Println("Connected to client with address", raddr, "in community", community)
			},
			OnDisconnect: func(raddr, community string, err interface{}) {
				log.Println("Disconnected from client with address", raddr, "in community", community)
			},
		},
		ctx,
	)

	if err := signaler.Open(); err != nil {
		panic(err)
	}

	s := make(chan os.Signal)
	signal.Notify(s, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-s

		log.Println("Gracefully shutting down signaling server")

		go func() {
			<-s

			log.Println("Forcing shutdown of signaling server")

			cancel()

			if err := signaler.Close(); err != nil {
				panic(err)
			}
		}()

		if err := signaler.Close(); err != nil {
			panic(err)
		}

		cancel()
	}()

	defer func() {
		if err := signaler.Close(); err != nil {
			panic(err)
		}
	}()

	log.Println("Listening on address", addr.String())

	if err := signaler.Wait(); err != nil {
		panic(err)
	}
}
