package persisters

import (
	"context"

	"github.com/pojntfx/webrtcfd/internal/db/sqlite/migrations/communities"
	models "github.com/pojntfx/webrtcfd/internal/db/sqlite/models/communities"
	migrate "github.com/rubenv/sql-migrate"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

//go:generate sqlboiler sqlite3 -o ../../internal/db/sqlite/models/communities -c ../../configs/sqlboiler/communities.yaml
//go:generate go-bindata -pkg communities -o ../../internal/db/sqlite/migrations/communities/migrations.go ../../db/sqlite/migrations/communities

type CommunitiesPersister struct {
	sqlite *SQLite

	root              string
	rootIsEmptyString bool
}

func NewCommunitiesPersister(dbPath string) *CommunitiesPersister {
	return &CommunitiesPersister{
		&SQLite{
			DBPath: dbPath,
			Migrations: migrate.AssetMigrationSource{
				Asset:    communities.Asset,
				AssetDir: communities.AssetDir,
				Dir:      "../../db/sqlite/migrations/communities",
			},
		},
		"",
		false,
	}
}

func (p *CommunitiesPersister) Open() error {
	return p.sqlite.Open()
}

func (p *CommunitiesPersister) CreateCommunity(ctx context.Context, id string, password string, persistent bool) error {
	community := &models.Community{
		ID:         id,
		Password:   password,
		Persistent: persistent,
	}

	return community.Insert(ctx, p.sqlite.DB, boil.Infer())
}

func (p *CommunitiesPersister) DeleteCommunity(ctx context.Context, id string) error {
	if _, err := models.Communities(qm.Where(models.CommunityColumns.ID+"= ?", id)).DeleteAll(ctx, p.sqlite.DB); err != nil {
		return err
	}

	return nil
}
