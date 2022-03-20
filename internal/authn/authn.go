package authn

type Authn interface {
	Validate(token string) bool
}
