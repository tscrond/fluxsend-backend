package repo

import (
	"context"
	"database/sql"

	sqlc "github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
)

type Repository interface {
	IsInitialized() bool
	Close() error
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
	Queries() *sqlc.Queries
}
