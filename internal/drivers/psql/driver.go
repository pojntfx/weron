package psql

import (
	"database/sql"

	_ "github.com/lib/pq"
	migrate "github.com/rubenv/sql-migrate"
)

type PSQL struct {
	DBUrl      string
	Migrations migrate.MigrationSource

	DB *sql.DB
}

func (s *PSQL) RunMigrations() error {
	// Connect to the DB
	db, err := sql.Open("postgres", s.DBUrl)
	if err != nil {
		return err
	}

	// Configure the db
	db.SetMaxOpenConns(1) // Prevent "database locked" errors
	s.DB = db

	// Run migrations if set
	if s.Migrations != nil {
		if _, err := migrate.Exec(s.DB, "postgres", s.Migrations, migrate.Up); err != nil {
			return err
		}
	}

	return nil
}
