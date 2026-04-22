package repo

import (
	"context"
	"database/sql"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"github.com/tscrond/fluxsend-backend/internal/repo/migrationhelper"
	sqlc "github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
)

type Repository struct {
	db      *sql.DB
	Queries *sqlc.Queries
}

func NewRepository(db *sql.DB) (*Repository, error) {

	queries := sqlc.New(db)

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		log.Println("driver error", err)
	}

	entries, err := os.ReadDir("./internal/repo/migrations")
	if err != nil {
		log.Fatalf("Can't read migrations dir: %v", err)
	}
	for _, e := range entries {
		log.Printf("Found migration file: %s", e.Name())
	}

	m, err := migrate.NewWithDatabaseInstance("file://internal/repo/migrations", "postgres", driver)
	if err != nil {
		log.Println("error creating migrations:", err)
		return nil, err
	}

	migrations, err := migrationhelper.NewMigrator(db, m, queries)
	if err != nil {
		log.Println("error running migration helper:", err)
		return nil, err
	}

	if err := migrations.Migrate(); err != nil {
		return nil, err
	}

	return &Repository{
		db:      db,
		Queries: queries,
	}, nil
}

func (repo *Repository) Close() error {
	if repo != nil {
		return repo.db.Close()
	}
	return nil
}

func (repo *Repository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return repo.db.BeginTx(ctx, nil)
}
