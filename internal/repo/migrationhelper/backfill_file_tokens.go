package migrationhelper

import (
	"context"
	"database/sql"
	"log"

	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"github.com/tscrond/fluxsend-backend/pkg"
)

func (m *Migrator) BackfillPrivateDownloadTokens() error {

	ctx := context.Background()

	fileIDs, err := m.query.ListFileIDsWithoutPrivateToken(ctx)
	if err != nil {
		return err
	}

	if len(fileIDs) == 0 {
		log.Println("No need to run backfill!")
		return nil
	}

	for _, id := range fileIDs {

		token, err := pkg.GenerateSecureTokenFromID(int64(id))
		if err != nil {
			return err
		}

		err = m.query.UpdatePrivateDownloadToken(ctx, sqlc.UpdatePrivateDownloadTokenParams{
			ID:                   id,
			PrivateDownloadToken: sql.NullString{Valid: true, String: token},
		})
		if err != nil {
			return err
		}
	}

	var exists bool
	err = m.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_constraint
			WHERE conname = 'files_private_token_unique'
		);
	`).Scan(&exists)
	if err != nil {
		return err
	}

	if !exists {
		_, err := m.db.ExecContext(ctx, `
			ALTER TABLE files
			ALTER COLUMN private_download_token SET NOT NULL,
			ADD CONSTRAINT files_private_token_unique UNIQUE (private_download_token);
		`)
		if err != nil {
			return err
		}
	}

	return nil
}
