package migrationhelper

import (
	"database/sql"
	"errors"
	"log"

	"github.com/golang-migrate/migrate/v4"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
)

type Migrator struct {
	db       *sql.DB
	migrator *migrate.Migrate
	query    *sqlc.Queries
}

func NewMigrator(db *sql.DB, m *migrate.Migrate, queries *sqlc.Queries) (*Migrator, error) {
	if db == nil {
		return nil, errors.New("db object is nil")
	}
	if m == nil {
		return nil, errors.New("migration object is nil")
	}
	if queries == nil {
		return nil, errors.New("queries object is nil")
	}

	return &Migrator{
		db:       db,
		migrator: m,
		query:    queries,
	}, nil
}

func (m *Migrator) Migrate() error {
	if err := m.PerformStandardMigrations(); err != nil {
		return err
	}
	if err := m.PerformCustomMigrations(); err != nil {
		return err
	}

	log.Println("All migrations succeeded!")

	return nil
}

func (m *Migrator) PerformStandardMigrations() error {
	log.Println("running migrations...")
	if err := m.migrator.Up(); err != nil && err != migrate.ErrNoChange {
		log.Println("error running up migration", err)
		return err
	}
	return nil
}

func (m *Migrator) PerformCustomMigrations() error {
	if err := m.BackfillPrivateDownloadTokens(); err != nil {
		return err
	}

	return nil
}
