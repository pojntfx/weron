package authn

import "context"

type Authn interface {
	Open(context.Context) error
	Validate(token string) error
}
