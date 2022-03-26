package oidc

import (
	"context"

	ioidc "github.com/coreos/go-oidc/v3/oidc"
)

type Authn struct {
	issuer   string
	clientID string

	ctx context.Context

	verifier *ioidc.IDTokenVerifier
}

func NewAuthn(issuer string, clientID string) *Authn {
	return &Authn{
		issuer:   issuer,
		clientID: clientID,
	}
}

func (a *Authn) Open(ctx context.Context) error {
	provider, err := ioidc.NewProvider(ctx, a.issuer)
	if err != nil {
		return err
	}

	a.ctx = ctx
	a.verifier = provider.Verifier(&ioidc.Config{ClientID: a.clientID})

	return nil
}

func (a *Authn) Validate(_, token string) error {
	if _, err := a.verifier.Verify(a.ctx, token); err != nil {
		return err
	}

	return nil
}
