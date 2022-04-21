package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pojntfx/webrtcfd/pkg/wrtcconn"
)

var (
	errMissingCommunity = errors.New("missing community")
	errMissingPassword  = errors.New("missing password")

	errMissingKey = errors.New("missing key")
)

const (
	acklen = 100
)

func main() {
	raddr := flag.String("raddr", "wss://webrtcfd.herokuapp.com/", "Remote address")
	timeout := flag.Duration("timeout", time.Second*10, "Time to wait for connections")
	community := flag.String("community", "", "ID of community to join")
	password := flag.String("password", "", "Password for community")
	key := flag.String("key", "", "Encryption key for community")
	ice := flag.String("ice", "stun:stun.l.google.com:19302", "Comma-separated list of STUN servers (in format stun:host:port) and TURN servers to use (in format username:credential@turn:host:port) (i.e. username:credential@turn:global.turn.twilio.com:3478?transport=tcp)")
	server := flag.Bool("server", false, "Act as a server")
	packetLength := flag.Int("packet-length", 1000, "Size of packet to send")
	packetCount := flag.Int("packet-count", 1000, "Amount of packets to send before waiting for acknowledgement")
	relay := flag.Bool("force-relay", false, "Force usage of TURN servers")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")

	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if strings.TrimSpace(*community) == "" {
		panic(errMissingCommunity)
	}

	if strings.TrimSpace(*password) == "" {
		panic(errMissingPassword)
	}

	if strings.TrimSpace(*key) == "" {
		panic(errMissingKey)
	}

	fmt.Printf("\r\u001b[0K.%v\n", *raddr)

	u, err := url.Parse(*raddr)
	if err != nil {
		panic(err)
	}

	q := u.Query()
	q.Set("community", *community)
	q.Set("password", *password)
	u.RawQuery = q.Encode()

	adapter := wrtcconn.NewAdapter(
		u.String(),
		*key,
		strings.Split(*ice, ","),
		[]string{"speedtest"},
		&wrtcconn.AdapterConfig{
			Timeout:    *timeout,
			Verbose:    *verbose,
			ForceRelay: *relay,
		},
		ctx,
	)

	ids, err := adapter.Open()
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := adapter.Close(); err != nil {
			panic(err)
		}
	}()

	totalTransferred := 0
	totalStart := time.Now()
	ready := false

	minSpeed := math.MaxFloat64
	maxSpeed := float64(0)

	s := make(chan os.Signal)
	signal.Notify(s, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-s

		if ready {
			totalDuration := time.Since(totalStart)

			totalSpeed := (float64(totalTransferred) / totalDuration.Seconds()) / 1000000

			fmt.Printf("Average: %.3f MB/s (%.3f Mb/s) (%v MB written in %v) Min: %.3f MB/s Max: %.3f MB/s\n", totalSpeed, totalSpeed*8, totalTransferred/1000000, totalDuration, minSpeed, maxSpeed)
		}

		os.Exit(0)
	}()

	errs := make(chan error)
	id := ""
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errs:
			panic(err)
		case id = <-ids:
			fmt.Printf("\r\u001b[0K%v.", id)
		case peer := <-adapter.Accept():
			fmt.Printf("\r\u001b[0K+%v@%v\n", peer.PeerID, peer.ChannelID)

			ready = true
			totalStart = time.Now()

			if *server {
				go func() {
					defer func() {
						fmt.Printf("\r\u001b[0K-%v@%v\n", peer.PeerID, peer.ChannelID)
					}()

					for {
						start := time.Now()

						written := 0
						for i := 0; i < *packetCount; i++ {
							buf := make([]byte, *packetLength)
							if _, err := rand.Read(buf); err != nil {
								errs <- err

								return
							}

							n, err := peer.Conn.Write(buf)
							if err != nil {
								errs <- err

								return
							}

							written += n
						}

						buf := make([]byte, acklen)
						if _, err := peer.Conn.Read(buf); err != nil {
							errs <- err

							return
						}

						duration := time.Since(start)

						speed := (float64(written) / duration.Seconds()) / 1000000

						if speed < float64(minSpeed) {
							minSpeed = speed
						}

						if speed > float64(maxSpeed) {
							maxSpeed = speed
						}

						log.Printf("%.3f MB/s (%.3f Mb/s) (%v MB written in %v)", speed, speed*8, written/1000000, duration)

						totalTransferred += written
					}
				}()
			} else {
				go func() {
					defer func() {
						fmt.Printf("\r\u001b[0K-%v@%v\n", peer.PeerID, peer.ChannelID)
					}()

					for {
						start := time.Now()

						read := 0
						for i := 0; i < *packetCount; i++ {
							buf := make([]byte, *packetLength)

							n, err := peer.Conn.Read(buf)
							if err != nil {
								errs <- err

								return
							}

							read += n
						}

						if _, err := peer.Conn.Write(make([]byte, acklen)); err != nil {
							errs <- err

							return
						}

						duration := time.Since(start)

						speed := (float64(read) / duration.Seconds()) / 1000000

						if speed < float64(minSpeed) {
							minSpeed = speed
						}

						if speed > float64(maxSpeed) {
							maxSpeed = speed
						}

						log.Printf("%.3f MB/s (%.3f Mb/s) (%v MB read in %v)", speed, speed*8, read/1000000, duration)

						totalTransferred += read
					}
				}()
			}
		}
	}
}
