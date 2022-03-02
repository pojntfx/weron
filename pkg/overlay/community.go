package overlay

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strings"
)

var (
	ErrMissingAuth        = errors.New("missing auth")
	ErrMissingPeerID      = errors.New("missing peer ID")
	ErrMissingPassword    = errors.New("missing password")
	ErrMissingCommunityID = errors.New("missing community ID")
)

type Community struct {
	uri string
	ice []string
}

func NewCommunity(
	uri string, // URI in format wss://mypeerid:mypassword@signaler.pojtinger.com#mycommunity
	ice []string,
) *Community {
	return &Community{
		uri: uri,
		ice: ice,
	}
}

func (c *Community) Join(ctx context.Context) error {
	uri, err := url.Parse(c.uri)
	if err != nil {
		return err
	}

	if uri.User == nil {
		return ErrMissingAuth
	}

	peerID := uri.User.Username()
	if strings.TrimSpace(peerID) == "" {
		return ErrMissingPeerID
	}

	password, set := uri.User.Password()
	if !set || strings.TrimSpace(password) == "" {
		return ErrMissingPassword
	}

	communityID := uri.Fragment
	if strings.TrimSpace(communityID) == "" {
		return ErrMissingCommunityID
	}

	return nil
}

func (c *Community) Leave() error {
	return nil
}

func (c *Community) Accept() (string, io.ReadWriter, error) {
	return "", nil, nil
}
