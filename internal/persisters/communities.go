package persisters

import (
	"context"
	"database/sql"
	"errors"

	"github.com/pojntfx/webrtcfd/internal/db/sqlite/migrations/communities"
	models "github.com/pojntfx/webrtcfd/internal/db/sqlite/models/communities"
	migrate "github.com/rubenv/sql-migrate"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"golang.org/x/crypto/bcrypt"
)

//go:generate sqlboiler sqlite3 -o ../../internal/db/sqlite/models/communities -c ../../configs/sqlboiler/communities.yaml
//go:generate go-bindata -pkg communities -o ../../internal/db/sqlite/migrations/communities/migrations.go ../../db/sqlite/migrations/communities

var (
	ErrWrongPassword = errors.New("wrong password")
)

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

func (p *CommunitiesPersister) AddClientsToCommunity(
	ctx context.Context,
	communityID string,
	password string,
	persistent bool,
) error {
	tx, err := p.sqlite.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		if err := tx.Rollback(); err != nil {
			return err
		}

		return err
	}

	community, err := models.FindCommunity(ctx, tx, communityID)
	if err != nil {
		if err == sql.ErrNoRows {
			community = &models.Community{
				ID:         communityID,
				Password:   string(hashedPassword),
				Clients:    1,
				Persistent: persistent,
			}

			if err := community.Insert(ctx, tx, boil.Infer()); err != nil {
				if err := tx.Rollback(); err != nil {
					return err
				}

				return err
			}

			return tx.Commit()
		} else {
			if err := tx.Rollback(); err != nil {
				return err
			}

			return err
		}
	}

	if bcrypt.CompareHashAndPassword([]byte(community.Password), []byte(password)) != nil {
		if err := tx.Rollback(); err != nil {
			return err
		}

		return ErrWrongPassword
	}

	community.Clients += 1

	if _, err := community.Update(ctx, tx, boil.Infer()); err != nil {
		if err := tx.Rollback(); err != nil {
			return err
		}

		return err
	}

	return tx.Commit()
}

func (p *CommunitiesPersister) RemoveClientFromCommunity(
	ctx context.Context,
	communityID string,
) error {
	tx, err := p.sqlite.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	community, err := models.FindCommunity(ctx, tx, communityID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil // No-op
		}

		if err := tx.Rollback(); err != nil {
			return err
		}

		return err
	}

	community.Clients -= 1
	if community.Clients <= 0 {
		if !community.Persistent {
			if _, err := community.Delete(ctx, tx); err != nil {
				if err := tx.Rollback(); err != nil {
					return err
				}

				return err
			}

			return tx.Commit()
		}

		community.Clients = 0
	}

	if _, err := community.Update(ctx, tx, boil.Infer()); err != nil {
		if err := tx.Rollback(); err != nil {
			return err
		}

		return err
	}

	return tx.Commit()
}
