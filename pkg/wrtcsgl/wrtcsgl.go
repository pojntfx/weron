package wrtcsgl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	rediserr "github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	jsoniter "github.com/json-iterator/go"
	"github.com/pojntfx/weron/internal/authn"
	"github.com/pojntfx/weron/internal/authn/basic"
	"github.com/pojntfx/weron/internal/authn/oidc"
	"github.com/pojntfx/weron/internal/brokers"
	"github.com/pojntfx/weron/internal/brokers/process"
	"github.com/pojntfx/weron/internal/brokers/redis"
	"github.com/pojntfx/weron/internal/persisters"
	"github.com/pojntfx/weron/internal/persisters/memory"
	"github.com/pojntfx/weron/internal/persisters/psql"
)

var (
	errMissingCommunity = errors.New("missing community")
	errMissingPassword  = errors.New("missing password")

	upgrader = websocket.Upgrader{}

	json = jsoniter.ConfigCompatibleWithStandardLibrary
)

type connection struct {
	conn   *websocket.Conn
	closer chan struct{}
}

type SignalerConfig struct {
	Heartbeat           time.Duration
	Cleanup             bool
	EphermalCommunities bool
	APIUsername         string
	APIPassword         string
	OIDCIssuer          string
	OIDCClientID        string

	OnConnect    func(raddr string, community string)
	OnDisconnect func(raddr string, community string, err interface{})
}

type Signaler struct {
	laddr       string
	postgresURL string
	redisURL    string
	config      *SignalerConfig
	ctx         context.Context

	errs            chan error
	connectionsLock sync.Mutex
	connections     map[string]map[string]connection
	db              persisters.CommunitiesPersister
	broker          brokers.CommunitiesBroker
	srv             *http.Server
	closeKicks      func() error
}

func NewSignaler(
	laddr string,
	dbURL string,
	brokerURL string,
	config *SignalerConfig,
	ctx context.Context,
) *Signaler {
	if config == nil {
		config = &SignalerConfig{}
	}

	return &Signaler{
		laddr:       laddr,
		postgresURL: dbURL,
		redisURL:    brokerURL,
		config:      config,
		ctx:         ctx,

		errs: make(chan error),
	}
}

func (s *Signaler) Open() error {
	log.Trace().Msg("Opening signaler")

	addr, err := net.ResolveTCPAddr("tcp", s.laddr)
	if err != nil {
		return err
	}

	managementAPIEnabled := true
	if (strings.TrimSpace(s.config.OIDCIssuer) == "" && strings.TrimSpace(s.config.OIDCClientID) == "") && strings.TrimSpace(s.config.APIPassword) == "" {
		managementAPIEnabled = false

		log.Debug().Msg("API password not set, disabling management API")
	}

	if strings.TrimSpace(s.postgresURL) == "" {
		s.db = memory.NewCommunitiesPersister()
	} else {
		s.db = psql.NewCommunitiesPersister()
	}

	if err := s.db.Open(s.postgresURL); err != nil {
		return err
	}

	if s.config.Cleanup {
		if err := s.db.Cleanup(s.ctx); err != nil {
			return err
		}
	}

	if strings.TrimSpace(s.redisURL) == "" {
		s.broker = process.NewCommunitiesBroker()
	} else {
		s.broker = redis.NewCommunitiesBroker()
	}

	if err := s.broker.Open(s.ctx, s.redisURL); err != nil {
		return err
	}

	var authn authn.Authn
	if strings.TrimSpace(s.config.OIDCIssuer) == "" && strings.TrimSpace(s.config.OIDCClientID) == "" {
		authn = basic.NewAuthn(s.config.APIUsername, s.config.APIPassword)
	} else {
		authn = oidc.NewAuthn(s.config.OIDCIssuer, s.config.OIDCClientID)
	}

	if err := authn.Open(s.ctx); err != nil {
		return err
	}

	s.srv = &http.Server{Addr: addr.String()}

	s.connections = map[string]map[string]connection{}

	kicks, closeKicks := s.broker.SubscribeToKicks(s.ctx, s.errs)
	s.closeKicks = closeKicks

	s.srv.Handler = http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		raddr := uuid.New().String()

		defer func() {
			err := recover()

			switch err {
			case nil:
				log.Debug().
					Str("address", raddr).
					Msg("Closed connection for client")
			case http.StatusUnauthorized:
				fallthrough
			case http.StatusNotImplemented:
				log.Debug().
					Err(err.(error)).
					Str("address", raddr).
					Msg("Closed connection for client")
			case http.StatusNotFound:
				log.Debug().
					Err(err.(error)).
					Str("address", raddr).
					Msg("Closed connection for client")
			default:
				rw.WriteHeader(http.StatusInternalServerError)

				log.Debug().
					Err(err.(error)).
					Str("address", raddr).
					Msg("Closed connection for client")
			}
		}()

		switch r.Method {
		case http.MethodGet:
			community := r.URL.Query().Get("community")
			if strings.TrimSpace(community) == "" {
				if !managementAPIEnabled {
					rw.WriteHeader(http.StatusNotImplemented)

					panic(http.StatusNotImplemented)
				}

				// List communities
				u, p, ok := r.BasicAuth()
				if err := authn.Validate(u, p); !ok || err != nil {
					rw.WriteHeader(http.StatusUnauthorized)

					panic(http.StatusUnauthorized)
				}

				pc, err := s.db.GetCommunities(s.ctx)
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

			if err := s.db.AddClientsToCommunity(s.ctx, community, password, s.config.EphermalCommunities); err != nil {
				if err == persisters.ErrWrongPassword || err == persisters.ErrEphermalCommunitiesDisabled {
					rw.WriteHeader(http.StatusUnauthorized)

					panic(http.StatusUnauthorized)
				} else {
					panic(err)
				}
			}

			defer func() {
				if err := s.db.RemoveClientFromCommunity(s.ctx, community); err != nil {
					panic(err)
				}
			}()

			conn, err := upgrader.Upgrade(rw, r, nil)
			if err != nil {
				panic(err)
			}

			defer func() {
				s.connectionsLock.Lock()
				delete(s.connections[community], raddr)
				if len(s.connections[community]) <= 0 {
					delete(s.connections, community)
				}
				s.connectionsLock.Unlock()

				log.Debug().
					Str("address", raddr).
					Str("community", community).
					Msg("Disconnected from client")

				if s.config.OnDisconnect != nil {
					s.config.OnDisconnect(raddr, community, err)
				}

				if err := conn.Close(); err != nil {
					panic(err)
				}
			}()

			s.connectionsLock.Lock()
			if _, exists := s.connections[community]; !exists {
				s.connections[community] = map[string]connection{}
			}
			s.connections[community][raddr] = connection{
				conn:   conn,
				closer: make(chan struct{}),
			}
			s.connectionsLock.Unlock()

			log.Debug().
				Str("address", raddr).
				Str("community", community).
				Msg("Connected from client")

			if s.config.OnConnect != nil {
				s.config.OnConnect(raddr, community)
			}

			if err := conn.SetReadDeadline(time.Now().Add(s.config.Heartbeat)); err != nil {
				panic(err)
			}
			conn.SetPongHandler(func(string) error {
				return conn.SetReadDeadline(time.Now().Add(s.config.Heartbeat))
			})

			pings := time.NewTicker(s.config.Heartbeat / 2)
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

					log.Debug().
						Str("address", raddr).
						Str("community", community).
						Int("type", messageType).
						Msg("Received message")

					if err := s.broker.PublishInput(s.ctx, brokers.Input{
						Raddr:       raddr,
						MessageType: messageType,
						P:           p,
					}, community); err != nil {
						errs <- err

						return
					}
				}
			}()

			inputs, closeInputs := s.broker.SubscribeToInputs(s.ctx, errs, community)
			defer func() {
				if err := closeInputs(); err != nil {
					panic(err)
				}
			}()

			for {
				select {
				case <-s.connections[community][raddr].closer:
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

					if err := conn.SetWriteDeadline(time.Now().Add(s.config.Heartbeat)); err != nil {
						panic(err)
					}
				case <-pings.C:
					log.Debug().
						Str("address", raddr).
						Str("community", community).
						Msg("Sending ping to client")

					if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
						panic(err)
					}

					if err := conn.SetWriteDeadline(time.Now().Add(s.config.Heartbeat)); err != nil {
						panic(err)
					}
				}
			}
		case http.MethodPost:
			if !managementAPIEnabled {
				rw.WriteHeader(http.StatusNotImplemented)

				panic(http.StatusNotImplemented)
			}

			// Create persistent community
			u, p, ok := r.BasicAuth()
			if err := authn.Validate(u, p); !ok || err != nil {
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

			c, err := s.db.CreatePersistentCommunity(s.ctx, community, password)
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
			if !managementAPIEnabled {
				rw.WriteHeader(http.StatusNotImplemented)

				panic(http.StatusNotImplemented)
			}

			// Delete persistent community
			u, p, ok := r.BasicAuth()
			if err := authn.Validate(u, p); !ok || err != nil {
				rw.WriteHeader(http.StatusUnauthorized)

				panic(http.StatusUnauthorized)
			}

			community := r.URL.Query().Get("community")
			if strings.TrimSpace(community) == "" {
				panic(errMissingCommunity)
			}

			if err := s.db.DeleteCommunity(s.ctx, community); err != nil {
				if err == sql.ErrNoRows {
					rw.WriteHeader(http.StatusNotFound)

					panic(http.StatusNotFound)
				} else {
					panic(err)
				}
			}

			if err := s.broker.PublishKick(s.ctx, brokers.Kick{
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

			s.connectionsLock.Lock()
			c, ok := s.connections[kick.Community]
			if !ok {
				s.connectionsLock.Unlock()

				continue
			}
			s.connectionsLock.Unlock()

			for _, conn := range c {
				close(conn.closer)
			}
		}
	}()

	go func() {
		if err := s.srv.ListenAndServe(); err != nil {
			if err == http.ErrServerClosed {
				close(s.errs)

				return
			}

			s.errs <- err

			return
		}
	}()

	return nil
}

func (s *Signaler) Close() error {
	log.Trace().Msg("Closing signaler")

	s.connectionsLock.Lock()
	defer s.connectionsLock.Unlock()
	for c := range s.connections {
		for range s.connections[c] {
			if err := s.db.RemoveClientFromCommunity(s.ctx, c); err != nil {
				return err
			}
		}
	}

	if err := s.closeKicks(); err != nil {
		if err != context.Canceled {
			return err
		}
	}

	if err := s.broker.Close(); err != nil {
		if err != context.Canceled && err != rediserr.ErrClosed {
			return err
		}
	}

	if err := s.srv.Shutdown(s.ctx); err != nil {
		if err != context.Canceled {
			return err
		}
	}

	return nil
}

func (s *Signaler) Wait() error {
	for err := range s.errs {
		if err != nil {
			return err
		}
	}

	return nil
}
