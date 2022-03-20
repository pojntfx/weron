package basic

type Authn struct {
	password string
}

func NewAuthn(password string) *Authn {
	return &Authn{
		password: password,
	}
}

func (a *Authn) Validate(token string) bool {
	return token == a.password
}
