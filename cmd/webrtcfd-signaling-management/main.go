package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/pojntfx/webrtcfd/pkg/wrtcmgr"
)

var (
	errMissingCommunity   = errors.New("missing community")
	errMissingPassword    = errors.New("missing password")
	errMissingAPIPassword = errors.New("missing API password")
	errMissingAPIUsername = errors.New("missing API username")
)

func main() {
	raddr := flag.String("raddr", "https://webrtcfd.herokuapp.com/", "Remote address")
	apiUsername := flag.String("api-username", "admin", "Username for the management API (can also be set using the API_USERNAME env variable). Not used if OIDC token is provided.")
	apiPassword := flag.String("api-password", "", "Password (or OIDC token) for the management API (can also be set using the API_PASSWORD env variable)")
	community := flag.String("community", "", "ID of community to create")
	password := flag.String("password", "", "Password for community")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `Usage of %s:

Usage:
  %s [args] <command>

Available Commands:
  list		List persistent and ephermal communities
  create	Create a persistent community
  delete	Delete a persistent or ephermal community

Flags:
`, os.Args[0], os.Args[0])

		flag.PrintDefaults()
	}

	flag.Parse()

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

	if strings.TrimSpace(*apiPassword) == "" {
		panic(errMissingAPIPassword)
	}

	if strings.TrimSpace(*apiUsername) == "" {
		panic(errMissingAPIUsername)
	}

	if flag.NArg() <= 0 {
		flag.Usage()

		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager := wrtcmgr.NewManager(
		*raddr,
		*apiUsername,
		*apiPassword,
		ctx,
	)

	switch flag.Args()[0] {
	case "list":
		c, err := manager.ListCommunities()
		if err != nil {
			panic(err)
		}

		w := csv.NewWriter(os.Stdout)
		defer w.Flush()

		if err := w.Write([]string{"id", "clients", "persistent"}); err != nil {
			panic(err)
		}

		for _, community := range c {
			if err := w.Write([]string{community.ID, fmt.Sprintf("%v", community.Clients), fmt.Sprintf("%v", community.Persistent)}); err != nil {
				panic(err)
			}
		}
	case "create":
		if strings.TrimSpace(*community) == "" {
			panic(errMissingCommunity)
		}

		if strings.TrimSpace(*password) == "" {
			panic(errMissingPassword)
		}

		c, err := manager.CreatePersistentCommunity(*community, *password)
		if err != nil {
			panic(err)
		}

		w := csv.NewWriter(os.Stdout)
		defer w.Flush()

		if err := w.Write([]string{"id", "clients", "persistent"}); err != nil {
			panic(err)
		}

		if err := w.Write([]string{c.ID, fmt.Sprintf("%v", c.Clients), fmt.Sprintf("%v", c.Persistent)}); err != nil {
			panic(err)
		}
	case "delete":
		if strings.TrimSpace(*community) == "" {
			panic(errMissingCommunity)
		}

		if err := manager.DeleteCommunity(*community); err != nil {
			panic(err)
		}
	default:
		panic("unknown command")
	}
}
