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
	"path"
	"strings"

	"github.com/pojntfx/webrtcfd/internal/persisters"
)

const (
	user = "user"
)

var (
	errMissingPassword          = errors.New("missing password")
	errMissingCommunityID       = errors.New("missing community ID")
	errMissingCommunityPassword = errors.New("missing community password")
)

func main() {
	raddr := flag.String("raddr", "https://webrtcfd.herokuapp.com/", "Remote address")
	password := flag.String("password", "", "Password for the management API (can also be set using the PASSWORD env variable)")
	communityID := flag.String("community-id", "", "ID of the community to create")
	communityPassword := flag.String("community-password", "", "Password for the community to create")
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

	if p := os.Getenv("PASSWORD"); p != "" {
		if *verbose {
			log.Println("Using password from PASSWORD env variable")
		}

		*password = p
	}

	if strings.TrimSpace(*password) == "" {
		panic(errMissingPassword)
	}

	if flag.NArg() <= 0 {
		flag.Usage()

		return
	}

	switch flag.Args()[0] {
	case "list":
		c := &http.Client{}

		req, err := http.NewRequest(http.MethodGet, *raddr, http.NoBody)
		if err != nil {
			panic(err)
		}
		req.SetBasicAuth(user, *password)

		res, err := c.Do(req)
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

		pc := []persisters.Community{}
		if err := json.Unmarshal(body, &pc); err != nil {
			panic(err)
		}

		w := csv.NewWriter(os.Stdout)
		defer w.Flush()

		if err := w.Write([]string{"id", "clients", "persistent"}); err != nil {
			panic(err)
		}

		for _, community := range pc {
			if err := w.Write([]string{community.ID, fmt.Sprintf("%v", community.Clients), fmt.Sprintf("%v", community.Persistent)}); err != nil {
				panic(err)
			}
		}
	case "create":
		if strings.TrimSpace(*communityID) == "" {
			panic(errMissingCommunityID)
		}

		if strings.TrimSpace(*communityPassword) == "" {
			panic(errMissingCommunityPassword)
		}

		c := &http.Client{}

		u, err := url.Parse(*raddr)
		if err != nil {
			panic(err)
		}
		u.Path = path.Join(u.Path, *communityID)

		q := u.Query()
		q.Set("password", *communityPassword)
		u.RawQuery = q.Encode()

		req, err := http.NewRequest(http.MethodPost, u.String(), http.NoBody)
		if err != nil {
			panic(err)
		}
		req.SetBasicAuth(user, *password)

		res, err := c.Do(req)
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

		pc := persisters.Community{}
		if err := json.Unmarshal(body, &pc); err != nil {
			panic(err)
		}

		w := csv.NewWriter(os.Stdout)
		defer w.Flush()

		if err := w.Write([]string{"id", "clients", "persistent"}); err != nil {
			panic(err)
		}

		if err := w.Write([]string{pc.ID, fmt.Sprintf("%v", pc.Clients), fmt.Sprintf("%v", pc.Persistent)}); err != nil {
			panic(err)
		}
	case "delete":
		if strings.TrimSpace(*communityID) == "" {
			panic(errMissingCommunityID)
		}

		c := &http.Client{}

		u, err := url.Parse(*raddr)
		if err != nil {
			panic(err)
		}
		u.Path = path.Join(u.Path, *communityID)

		req, err := http.NewRequest(http.MethodDelete, u.String(), http.NoBody)
		if err != nil {
			panic(err)
		}
		req.SetBasicAuth(user, *password)

		res, err := c.Do(req)
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
