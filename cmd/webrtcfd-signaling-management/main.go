package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/pojntfx/webrtcfd/internal/persisters"
)

var (
	errMissingCommunity   = errors.New("missing community")
	errMissingPassword    = errors.New("missing password")
	errMissingAPIPassword = errors.New("missing API password")
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

	if flag.NArg() <= 0 {
		flag.Usage()

		return
	}

	switch flag.Args()[0] {
	case "list":
		hc := &http.Client{}

		req, err := http.NewRequest(http.MethodGet, *raddr, http.NoBody)
		if err != nil {
			panic(err)
		}
		req.SetBasicAuth(*apiUsername, *apiPassword)

		res, err := hc.Do(req)
		if err != nil {
			panic(err)
		}
		if res.Body != nil {
			defer res.Body.Close()
		}
		if res.StatusCode != http.StatusOK {
			panic(res.Status)
		}

		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			panic(err)
		}

		c := []persisters.Community{}
		if err := json.Unmarshal(body, &c); err != nil {
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

		hc := &http.Client{}

		u, err := url.Parse(*raddr)
		if err != nil {
			panic(err)
		}

		q := u.Query()
		q.Set("community", *community)
		q.Set("password", *password)
		u.RawQuery = q.Encode()

		req, err := http.NewRequest(http.MethodPost, u.String(), http.NoBody)
		if err != nil {
			panic(err)
		}
		req.SetBasicAuth(*apiUsername, *apiPassword)

		res, err := hc.Do(req)
		if err != nil {
			panic(err)
		}
		if res.Body != nil {
			defer res.Body.Close()
		}
		if res.StatusCode != http.StatusOK {
			panic(res.Status)
		}

		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			panic(err)
		}

		c := persisters.Community{}
		if err := json.Unmarshal(body, &c); err != nil {
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

		hc := &http.Client{}

		u, err := url.Parse(*raddr)
		if err != nil {
			panic(err)
		}

		q := u.Query()
		q.Set("community", *community)
		u.RawQuery = q.Encode()

		req, err := http.NewRequest(http.MethodDelete, u.String(), http.NoBody)
		if err != nil {
			panic(err)
		}
		req.SetBasicAuth(*apiUsername, *apiPassword)

		res, err := hc.Do(req)
		if err != nil {
			panic(err)
		}
		if res.Body != nil {
			defer res.Body.Close()
		}
		if res.StatusCode != http.StatusOK {
			panic(res.Status)
		}
	default:
		panic("unknown command")
	}
}
