package wrtctkn

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type TokenManager struct {
	oidcIssuer      string
	oidcClientID    string
	oidcRedirectURL string

	openURL func(string) error

	ctx context.Context
}

func NewTokenManager(
	oidcIssuer string,
	oidcClientID string,
	oidcRedirectURL string,

	openURL func(string) error,

	ctx context.Context,
) *TokenManager {
	return &TokenManager{
		oidcIssuer:      oidcIssuer,
		oidcClientID:    oidcClientID,
		oidcRedirectURL: oidcRedirectURL,

		openURL: openURL,

		ctx: ctx,
	}
}

func (t *TokenManager) GetToken() (string, error) {
	provider, err := oidc.NewProvider(t.ctx, t.oidcIssuer)
	if err != nil {
		return "", err
	}

	config := &oauth2.Config{
		ClientID:    t.oidcClientID,
		RedirectURL: t.oidcRedirectURL,
		Endpoint:    provider.Endpoint(),
		Scopes:      []string{oidc.ScopeOpenID},
	}

	u, err := url.Parse(t.oidcRedirectURL)
	if err != nil {
		return "", err
	}

	srv := &http.Server{Addr: u.Host}

	tokens := make(chan string)
	errs := make(chan error)

	srv.Handler = http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "text/html")

		if _, err := fmt.Fprint(rw, `<!DOCTYPE html><script>window.close()</script>`); err != nil {
			errs <- err

			return
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
		if err := srv.Shutdown(t.ctx); err != nil {
			panic(err)
		}
	}()

	authURL := config.AuthCodeURL(t.oidcRedirectURL)
	if err := t.openURL(authURL); err != nil {
		return "", err
	}

	for {
		select {
		case err := <-errs:
			return "", err
		case token := <-tokens:
			return token, nil
		}
	}
}
