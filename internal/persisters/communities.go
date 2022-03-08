package persisters

import (
	"github.com/pojntfx/webrtcfd/internal/db/sqlite/migrations/communities"
	migrate "github.com/rubenv/sql-migrate"
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
