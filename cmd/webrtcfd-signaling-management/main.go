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
	"os"
	"strings"

	"github.com/pojntfx/webrtcfd/internal/persisters"
)

var (
	errMissingPassword = errors.New("missing password")
)

func main() {
	raddr := flag.String("raddr", "https://webrtcfd.herokuapp.com/", "Remote address")
	password := flag.String("password", "", "Password for the management API (can also be set using the PASSWORD env variable)")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `Usage of %s:

Usage:
  %s <command>

Available Commands:
  list  List persistent communities

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
		req.SetBasicAuth("user", *password)

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

		pc := []persisters.PersistentCommunity{}
		if err := json.Unmarshal(body, &pc); err != nil {
			panic(err)
		}

		w := csv.NewWriter(os.Stdout)
		defer w.Flush()

		if err := w.Write([]string{"id", "clients"}); err != nil {
			panic(err)
		}

		for _, community := range pc {
			if err := w.Write([]string{community.ID, fmt.Sprintf("%v", community.Clients)}); err != nil {
				panic(err)
			}
		}
	default:
		panic("unknown command")
	}
}
