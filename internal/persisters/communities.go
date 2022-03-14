package persisters

import (
	"context"
	"database/sql"
	"errors"

	"github.com/pojntfx/webrtcfd/internal/db/sqlite/migrations/communities"
	models "github.com/pojntfx/webrtcfd/internal/db/sqlite/models/communities"
	migrate "github.com/rubenv/sql-migrate"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
	"golang.org/x/crypto/bcrypt"
)

//go:generate sqlboiler sqlite3 -o ../../internal/db/sqlite/models/communities -c ../../configs/sqlboiler/communities.yaml
//go:generate go-bindata -pkg communities -o ../../internal/db/sqlite/migrations/communities/migrations.go ../../db/sqlite/migrations/communities

var (
	ErrWrongPassword = errors.New("wrong password")
)

type Community struct {
	ID         string `json:"id"`
	Clients    int64  `json:"clients"`
	Persistent bool   `json:"persistent"`
}

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
	community string,
	password string,
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

	c, err := models.FindCommunity(ctx, tx, community)
	if err != nil {
		if err == sql.ErrNoRows {
			c = &models.Community{
				ID:         community,
				Password:   string(hashedPassword),
				Clients:    1,
				Persistent: false,
			}

			if err := c.Insert(ctx, tx, boil.Infer()); err != nil {
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

	if bcrypt.CompareHashAndPassword([]byte(c.Password), []byte(password)) != nil {
		if err := tx.Rollback(); err != nil {
			return err
		}

		return ErrWrongPassword
	}

	c.Clients += 1

	if _, err := c.Update(ctx, tx, boil.Infer()); err != nil {
		if err := tx.Rollback(); err != nil {
			return err
		}

		return err
	}

	return tx.Commit()
}

func (p *CommunitiesPersister) RemoveClientFromCommunity(
	ctx context.Context,
	community string,
) error {
	tx, err := p.sqlite.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	c, err := models.FindCommunity(ctx, tx, community)
	if err != nil {
		if err == sql.ErrNoRows {
			if err := tx.Rollback(); err != nil {
				return err
			}

			return nil // No-op
		}

		if err := tx.Rollback(); err != nil {
			return err
		}

		return err
	}

	c.Clients -= 1
	if c.Clients <= 0 {
		if !c.Persistent {
			if _, err := c.Delete(ctx, tx); err != nil {
				if err := tx.Rollback(); err != nil {
					return err
				}

				return err
			}

			return tx.Commit()
		}

		c.Clients = 0
	}

	if _, err := c.Update(ctx, tx, boil.Infer()); err != nil {
		if err := tx.Rollback(); err != nil {
			return err
		}

		return err
	}

	return tx.Commit()
}

func (p *CommunitiesPersister) Cleanup(
	ctx context.Context,
) error {
	tx, err := p.sqlite.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// Delete all ephermal communities
	if _, err := models.Communities(qm.Where(models.CommunityColumns.Persistent+"= ?", false)).DeleteAll(ctx, tx); err != nil {
		if err := tx.Rollback(); err != nil {
			return err
		}

		return err
	}

	// Set client count to 0 for all persistent communities
	if _, err := models.Communities(qm.Where(models.CommunityColumns.Persistent+"= ?", true)).UpdateAll(ctx, tx, models.M{models.CommunityColumns.Clients: 0}); err != nil {
		if err := tx.Rollback(); err != nil {
			return err
		}

		return err
	}

	return tx.Commit()
}

func (p *CommunitiesPersister) GetPersistent(
	ctx context.Context,
) ([]Community, error) {
	c, err := models.Communities().All(ctx, p.sqlite.DB)
	if err != nil {
		return nil, err
	}

	cc := []Community{}
	for _, community := range c {
		cc = append(cc, Community{
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
) (*models.Community, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	c := &models.Community{
		ID:         community,
		Password:   string(hashedPassword),
		Clients:    0,
		Persistent: true,
	}

	if err := c.Insert(ctx, p.sqlite.DB, boil.Infer()); err != nil {
		return nil, err
	}

	return c, nil
}

func (p *CommunitiesPersister) DeleteCommunity(
	ctx context.Context,
	community string,
) error {
	n, err := models.Communities(
		qm.Where(models.CommunityColumns.ID+"= ?", community),
	).DeleteAll(ctx, p.sqlite.DB)
	if err != nil {
		return err
	}

	if n <= 0 {
		return sql.ErrNoRows
	}

	return nil
}
