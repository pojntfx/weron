package persisters

import (
	"context"
	"errors"
)

var (
	ErrEphemeralCommunitiesDisabled = errors.New("creation of ephemeral communites is disabled")
)

type Community struct {
	ID         string `json:"id"`
	Clients    int    `json:"clients"`
	Persistent bool   `json:"persistent"`
}

type CommunitiesPersister interface {
	Open(dbURL string) error
	AddClientsToCommunity(
		ctx context.Context,
		community string,
		password string,
		upsert bool,
	) error
	RemoveClientFromCommunity(
		ctx context.Context,
		community string,
	) error
	Cleanup(
		ctx context.Context,
	) error
	GetCommunities(
		ctx context.Context,
	) ([]Community, error)
	CreatePersistentCommunity(
		ctx context.Context,
		community string,
		password string,
	) (*Community, error)
	DeleteCommunity(
		ctx context.Context,
		community string,
	) error
}
