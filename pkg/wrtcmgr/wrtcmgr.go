package wrtcmgr

import (
	"context"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"

	jsoniter "github.com/json-iterator/go"
	"github.com/pojntfx/weron/internal/persisters"
)

var (
	json = jsoniter.ConfigCompatibleWithStandardLibrary
)

// Manager manages a signaling server
type Manager struct {
	url      string
	username string
	password string
	ctx      context.Context
}

// NewManager creates the manager
func NewManager(
	url string,
	username string,
	password string,
	ctx context.Context,
) *Manager {
	return &Manager{
		url:      url,
		username: username,
		password: password,
		ctx:      ctx,
	}
}

// CreatePersistentCommunity creates a persistent community, which will not be automatically deleted after the last peer leaves
func (m *Manager) CreatePersistentCommunity(community string, password string) (*persisters.Community, error) {
	hc := &http.Client{}

	u, err := url.Parse(m.url)
	if err != nil {
		return nil, err
	}

	q := u.Query()
	q.Set("community", community)
	q.Set("password", password)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodPost, u.String(), http.NoBody)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(m.username, m.password)

	res, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	if res.Body != nil {
		defer res.Body.Close()
	}
	if res.StatusCode != http.StatusOK {
		return nil, errors.New(res.Status)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	c := persisters.Community{}
	if err := json.Unmarshal(body, &c); err != nil {
		return nil, err
	}

	return &c, nil
}

// ListCommunities queries all communities
func (m *Manager) ListCommunities() ([]persisters.Community, error) {
	u, err := url.Parse(m.url)
	if err != nil {
		return nil, err
	}

	hc := &http.Client{}

	req, err := http.NewRequest(http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(m.username, m.password)

	res, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	if res.Body != nil {
		defer res.Body.Close()
	}
	if res.StatusCode != http.StatusOK {
		return nil, errors.New(res.Status)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	c := []persisters.Community{}
	if err := json.Unmarshal(body, &c); err != nil {
		return nil, err
	}

	return c, nil
}

// DeleteCommunity deletes a community and kicks all peers that joined it
func (m *Manager) DeleteCommunity(community string) error {
	hc := &http.Client{}

	u, err := url.Parse(m.url)
	if err != nil {
		return err
	}

	q := u.Query()
	q.Set("community", community)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodDelete, u.String(), http.NoBody)
	if err != nil {
		return err
	}
	req.SetBasicAuth(m.username, m.password)

	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	if res.Body != nil {
		defer res.Body.Close()
	}
	if res.StatusCode != http.StatusOK {
		return errors.New(res.Status)
	}

	return nil
}
