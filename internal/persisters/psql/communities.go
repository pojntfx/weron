package psql

import (
	"context"
	"database/sql"

	"github.com/pojntfx/webrtcfd/internal/db/psql/migrations/communities"
	models "github.com/pojntfx/webrtcfd/internal/db/psql/models/communities"
	"github.com/pojntfx/webrtcfd/internal/drivers/psql"
	"github.com/pojntfx/webrtcfd/internal/persisters"
	migrate "github.com/rubenv/sql-migrate"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
	"golang.org/x/crypto/bcrypt"
)

//go:generate sqlboiler psql -o ../../../internal/db/psql/models/communities -c ../../../configs/sqlboiler/communities.yaml
//go:generate go-bindata -pkg communities -o ../../../internal/db/psql/migrations/communities/migrations.go ../../../db/psql/migrations/communities

type CommunitiesPersister struct {
	db *sql.DB
}

func NewCommunitiesPersister() *CommunitiesPersister {
	return &CommunitiesPersister{}
}

func (p *CommunitiesPersister) Open(dbURL string) error {
	db := &psql.PSQL{
		DBUrl: dbURL,
		Migrations: migrate.AssetMigrationSource{
			Asset:    communities.Asset,
			AssetDir: communities.AssetDir,
			Dir:      "../../../db/psql/migrations/communities",
		},
	}

	if err := db.RunMigrations(); err != nil {
		return err
	}

	p.db = db.DB

	return nil
}

func (p *CommunitiesPersister) AddClientsToCommunity(
	ctx context.Context,
	community string,
	password string,
	upsert bool,
) error {
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	})
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
			if !upsert {
				if err := tx.Rollback(); err != nil {
					return err
				}

				return persisters.ErrEphermalCommunitiesDisabled
			}

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

		return persisters.ErrWrongPassword
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
	tx, err := p.db.BeginTx(ctx, nil)
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
	tx, err := p.db.BeginTx(ctx, nil)
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

func (p *CommunitiesPersister) GetCommunities(
	ctx context.Context,
) ([]persisters.Community, error) {
	c, err := models.Communities().All(ctx, p.db)
	if err != nil {
		return nil, err
	}

	cc := []persisters.Community{}
	for _, community := range c {
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

	if err := c.Insert(ctx, p.db, boil.Infer()); err != nil {
		return nil, err
	}

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
	n, err := models.Communities(
		qm.Where(models.CommunityColumns.ID+"= ?", community),
	).DeleteAll(ctx, p.db)
	if err != nil {
		return err
	}

	if n <= 0 {
		return sql.ErrNoRows
	}

	return nil
}
