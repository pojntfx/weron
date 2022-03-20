package basic

import (
	"context"

	"github.com/pojntfx/webrtcfd/internal/persisters"
)

type Authn struct {
	password string
}

func NewAuthn(password string) *Authn {
	return &Authn{
		password: password,
	}
}

func (a *Authn) Open(context.Context) error {
	return nil
}

func (a *Authn) Validate(token string) error {
	if token != a.password {
		return persisters.ErrWrongPassword
	}

	return nil
}
