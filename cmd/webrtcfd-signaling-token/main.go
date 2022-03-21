package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/pkg/browser"
	"golang.org/x/oauth2"
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

	provider, err := oidc.NewProvider(ctx, *oidcIssuer)
	if err != nil {
		panic(err)
	}

	config := &oauth2.Config{
		ClientID:    *oidcClientID,
		RedirectURL: *oidcRedirectURL,
		Endpoint:    provider.Endpoint(),
		Scopes:      []string{oidc.ScopeOpenID},
	}

	u, err := url.Parse(*oidcRedirectURL)
	if err != nil {
		panic(err)
	}

	srv := &http.Server{Addr: u.Host}

	tokens := make(chan string)
	errs := make(chan error)

	srv.Handler = http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "text/html")

		if _, err := fmt.Fprint(rw, `<!DOCTYPE html><script>window.close()</script>`); err != nil {
			panic(err)
		}

		oauth2Token, err := config.Exchange(context.Background(), r.URL.Query().Get("code"))
		if err != nil {
			errs <- err

			return
		}

		tokens <- oauth2Token.Extra("id_token").(string)
	})

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
	defer func() {
		if err := srv.Shutdown(ctx); err != nil {
			panic(err)
		}
	}()

	authURL := config.AuthCodeURL(*oidcRedirectURL)
	browser.Stdout = os.Stderr // Prevent browser output from appearing in `stdout`
	if err := browser.OpenURL(authURL); err != nil {
		log.Printf(`Could not open browser, please open the following URL in your browser manually to authorize:
%v`, authURL)
	}

	for {
		select {
		case err := <-errs:
			panic(err)
		case token := <-tokens:
			log.Println("Successfully got the following OIDC access token:")

			fmt.Println(token)

			return
		}
	}
}
