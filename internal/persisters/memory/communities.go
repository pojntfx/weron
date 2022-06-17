package memory

import (
	"context"
	"database/sql"
	"errors"
	"sync"

	"github.com/pojntfx/go-auth-utils/pkg/authn"
	"github.com/pojntfx/weron/internal/persisters"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUniqueConstraintViolation = errors.New("unique constraint violation")
)

type Community struct {
	*persisters.Community
	password string
}

type CommunitiesPersister struct {
	lock        sync.Mutex
	communities []*Community
}

func NewCommunitiesPersister() *CommunitiesPersister {
	return &CommunitiesPersister{
		communities: []*Community{},
	}
}

func (p *CommunitiesPersister) Open(dbURL string) error {
	return nil
}

func (p *CommunitiesPersister) AddClientsToCommunity(
	ctx context.Context,
	community string,
	password string,
	upsert bool,
) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	var c *Community
	for _, candidate := range p.communities {
		if candidate.ID == community {
			c = candidate

			break
		}
	}

	if c == nil {
		p.communities = append(p.communities, &Community{
			password: string(hashedPassword),
			Community: &persisters.Community{
				ID:         community,
				Clients:    1,
				Persistent: false,
			},
		})

		return nil
	}

	if bcrypt.CompareHashAndPassword([]byte(c.password), []byte(password)) != nil {
		return authn.ErrWrongPassword
	}

	c.Clients += 1

	return nil
}

func (p *CommunitiesPersister) RemoveClientFromCommunity(
	ctx context.Context,
	community string,
) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	var c *Community
	for _, candidate := range p.communities {
		if candidate.ID == community {
			c = candidate

			break
		}
	}

	if c == nil {
		return sql.ErrNoRows
	}

	c.Clients -= 1
	if c.Clients <= 0 {
		if !c.Persistent {
			newCommunities := []*Community{}
			for _, candidate := range p.communities {
				if candidate.ID != community {
					newCommunities = append(newCommunities, candidate)
				}
			}

			p.communities = newCommunities

			return nil
		}

		c.Clients = 0
	}

	return nil
}

func (p *CommunitiesPersister) Cleanup(
	ctx context.Context,
) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	newCommunities := []*Community{}
	for _, candidate := range p.communities {
		// Delete all ephermal communities
		if candidate.Persistent {
			continue
		}

		// Set client count to 0 for all persistent communities
		candidate.Clients = 0

		newCommunities = append(newCommunities, candidate)
	}

	p.communities = newCommunities

	return nil
}

func (p *CommunitiesPersister) GetCommunities(
	ctx context.Context,
) ([]persisters.Community, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	cc := []persisters.Community{}
	for _, community := range p.communities {
		cc = append(cc, persisters.Community{
			ID:         community.ID,
			Clients:    community.Clients,
			Persistent: community.Persistent,
		})
	}

	return cc, nil
}

func (p *CommunitiesPersister) CreatePersistentCommunity(
	ctx context.Context,
	community string,
	password string,
) (*persisters.Community, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	var c *Community
	for _, candidate := range p.communities {
		if candidate.ID == community {
			c = candidate

			break
		}
	}

	if c != nil {
		return nil, ErrUniqueConstraintViolation
	}

	c = &Community{
		password: string(hashedPassword),
		Community: &persisters.Community{
			ID:         community,
			Clients:    0,
			Persistent: true,
		},
	}

	p.communities = append(p.communities, c)

	cc := &persisters.Community{
		ID:         c.ID,
		Clients:    c.Clients,
		Persistent: c.Persistent,
	}

	return cc, nil
}

func (p *CommunitiesPersister) DeleteCommunity(
	ctx context.Context,
	community string,
) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	newCommunities := []*Community{}
	n := 0
	for _, candidate := range p.communities {
		if candidate.ID != community {
			newCommunities = append(newCommunities, candidate)

			continue
		}

		n++
	}

	p.communities = newCommunities

	if n <= 0 {
		return sql.ErrNoRows
	}

	return nil
}
