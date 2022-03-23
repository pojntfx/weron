package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"encoding/json"

	"github.com/gorilla/websocket"
	websocketapi "github.com/pojntfx/webrtcfd/internal/api/websocket"
	"github.com/pojntfx/webrtcfd/internal/encryption"
)

var (
	errMissingCommunity = errors.New("missing community")
	errMissingPassword  = errors.New("missing password")

	errMissingKey = errors.New("missing key")
)

func main() {
	raddr := flag.String("raddr", "wss://webrtcfd.herokuapp.com/", "Remote address")
	timeout := flag.Duration("timeout", time.Second*10, "Time to wait for connections")
	community := flag.String("community", "", "ID of community to join")
	password := flag.String("password", "", "Password for community")
	key := flag.String("key", "", "Encryption key for community")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")

	flag.Parse()

	if strings.TrimSpace(*community) == "" {
		panic(errMissingCommunity)
	}

	if strings.TrimSpace(*password) == "" {
		panic(errMissingPassword)
	}

	if strings.TrimSpace(*key) == "" {
		panic(errMissingKey)
	}

	log.Println("Connecting to signaler with address", *raddr)

	u, err := url.Parse(*raddr)
	if err != nil {
		panic(err)
	}

	q := u.Query()
	q.Set("community", *community)
	q.Set("password", *password)
	u.RawQuery = q.Encode()

	lines := make(chan []byte)
	go func() {
		scan := bufio.NewScanner(os.Stdin)
		for scan.Scan() {
			lines <- scan.Bytes()
		}
	}()
	defer close(lines)

	for {
		func() {
			defer func() {
				if err := recover(); err != nil {
					log.Println("closed connection to signaler with address", *raddr+":", err, "(wrong password?)")
				}

				log.Println("Reconnecting to signaler with address", *raddr, "in", *timeout)

				time.Sleep(*timeout)
			}()

			ctx, cancel := context.WithTimeout(context.Background(), *timeout)
			defer cancel()

			conn, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), nil)
			if err != nil {
				panic(err)
			}

			defer func() {
				log.Println("Disconnected from signaler with address", *raddr)

				if err := conn.Close(); err != nil {
					panic(err)
				}
			}()

			log.Println("Connected to signaler with address", *raddr)

			inputs := make(chan []byte)
			errs := make(chan error)
			go func() {
				defer func() {
					close(inputs)
					close(errs)
				}()

				for {
					_, p, err := conn.ReadMessage()
					if err != nil {
						errs <- err

						return
					}

					inputs <- p
				}
			}()

			for {
				select {
				case err := <-errs:
					panic(err)
				case input := <-inputs:
					input, err = encryption.Decrypt(input, []byte(*key))
					if err != nil {
						if *verbose {
							log.Println("Could not decrypt message with length", len(input), "for signaler with address", conn.RemoteAddr(), "in community", *community+", skipping")
						}

						continue
					}

					if *verbose {
						log.Println("Received message with length", len(input), "from signaler with address", conn.RemoteAddr(), "in community", *community)
					}

					var message websocketapi.Message
					if err := json.Unmarshal(input, &message); err != nil {
						if *verbose {
							log.Println("Could not unmarshal message for signaler with address", conn.RemoteAddr(), "in community", *community+", skipping")
						}

						continue
					}

					switch message.Type {
					case websocketapi.TypeIntroduction:
						var introduction websocketapi.Introduction
						if err := json.Unmarshal(input, &introduction); err != nil {
							if *verbose {
								log.Println("Could not unmarshal introduction for signaler with address", conn.RemoteAddr(), "in community", *community+", skipping")
							}

							continue
						}

						fmt.Println(introduction)
					default:
						if *verbose {
							log.Println("Got message with unknown type", message.Type, "for signaler with address", conn.RemoteAddr(), "in community", *community+", skipping")
						}

						continue
					}

				case line := <-lines:
					line, err = encryption.Encrypt(line, []byte(*key))
					if err != nil {
						panic(err)
					}

					if *verbose {
						log.Println("Sending message with length", len(line), "to signaler with address", conn.RemoteAddr(), "in community", *community)
					}

					if err := conn.WriteMessage(websocket.TextMessage, line); err != nil {
						panic(err)
					}
				}
			}
		}()
	}
}
