//go:build !((darwin && amd64) || (darwin && arm64) || (freebsd && amd64) || (linux && arm) || (linux && arm64) || (linux && 386) || (linux && amd64) || (linux && s390x) || (windows && amd64))

package persisters

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
	migrate "github.com/rubenv/sql-migrate"
)

type SQLite struct {
	DBPath     string
	Migrations migrate.MigrationSource

	DB *sql.DB
}

func (s *SQLite) Open() error {
	// Create leading directories for database
	if err := os.MkdirAll(filepath.Dir(s.DBPath), os.ModePerm); err != nil {
		return err
	}

	// Open the DB
	db, err := sql.Open("sqlite3", s.DBPath)
	if err != nil {
		return err
	}

	// Configure the db
	db.SetMaxOpenConns(1) // Prevent "database locked" errors
	s.DB = db

	// Run migrations if set
	if s.Migrations != nil {
		if _, err := migrate.Exec(s.DB, "sqlite3", s.Migrations, migrate.Up); err != nil {
			return err
		}
	}

	return nil
}
