package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/pkg/browser"
	"github.com/pojntfx/webrtcfd/pkg/wrtctkn"
)

var (
	errMissingOIDCIssuer      = errors.New("missing OIDC issuer")
	errMissingOIDCClientID    = errors.New("missing OIDC client ID")
	errMissingOIDCRedirectURL = errors.New("missing OIDC redirect URL")
)

func main() {
	oidcIssuer := flag.String("oidc-issuer", "", "OIDC Issuer (i.e. https://pojntfx.eu.auth0.com/) (can also be set using the OIDC_ISSUER env variable)")
	oidcClientID := flag.String("oidc-client-id", "", "OIDC Client ID (i.e. myoidcclientid) (can also be set using the OIDC_CLIENT_ID env variable)")
	oidcRedirectURL := flag.String("oidc-redirect-url", "http://localhost:11337", "OIDC redirect URL")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")

	flag.Parse()

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

	if strings.TrimSpace(*oidcIssuer) == "" {
		panic(errMissingOIDCIssuer)
	}

	if strings.TrimSpace(*oidcClientID) == "" {
		panic(errMissingOIDCClientID)
	}

	if strings.TrimSpace(*oidcRedirectURL) == "" {
		panic(errMissingOIDCRedirectURL)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager := wrtctkn.NewTokenManager(
		*oidcIssuer,
		*oidcClientID,
		*oidcRedirectURL,

		func(s string) error {
			if err := browser.OpenURL(s); err != nil {
				log.Printf(`Could not open browser, please open the following URL in your browser manually to authorize:
%v`, s)
			}

			return nil
		},

		ctx,
	)

	token, err := manager.GetToken()
	if err != nil {
		panic(err)
	}

	log.Println("Successfully got the following OIDC access token:")

	fmt.Println(token)
}
