package service

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tscrond/fluxsend-backend/internal/apikeydata"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"go.uber.org/zap"
)

type apiKeyTestRepository struct {
	db      *sql.DB
	queries *sqlc.Queries
}

func (r *apiKeyTestRepository) IsInitialized() bool {
	return r.db != nil
}

func (r *apiKeyTestRepository) Close() error {
	return r.db.Close()
}

func (r *apiKeyTestRepository) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, opts)
}

func (r *apiKeyTestRepository) Queries() *sqlc.Queries {
	return r.queries
}

func TestAPIKeyService_CreateWorkspaceAPIKey_HappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	repository := &apiKeyTestRepository{db: db, queries: sqlc.New(db)}
	svc := NewAPIKeyService(zap.NewNop().Sugar(), repository)

	workspaceID := uuid.New()
	createdByID := uuid.New()
	apiKeyID := uuid.New()
	createdAt := time.Date(2026, 6, 4, 10, 11, 12, 0, time.UTC)

	data, err := apikeydata.NewWorkspaceAPIKeyData(
		"workspace key",
		"description",
		"generated-secret",
		workspaceID,
		createdByID,
		[]string{"workspaces:read", "workspaces:files:read"},
	)
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO api_keys (created_by_user_id, name, key_hash, description, status)")).
		WithArgs(createdByID, "workspace key", sqlmock.AnyArg(), sql.NullString{String: "description", Valid: true}, "active").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_by_user_id", "created_at", "name", "key_hash", "description", "status", "last_used_at", "revoked_at", "revoked_by_user_id"}).
			AddRow(apiKeyID, createdByID, createdAt, "workspace key", "hashed-key", "description", "active", nil, nil, nil))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO api_key_scopes (api_key_id, scope) VALUES ($1,$2)")).
		WithArgs(apiKeyID, "workspaces:read").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO api_key_scopes (api_key_id, scope) VALUES ($1,$2)")).
		WithArgs(apiKeyID, "workspaces:files:read").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO api_key_workspaces (api_key_id, workspace_id)")).
		WithArgs(apiKeyID, workspaceID).
		WillReturnRows(sqlmock.NewRows([]string{"api_key_id", "workspace_id"}).AddRow(apiKeyID, workspaceID))
	mock.ExpectCommit()

	result, err := svc.CreateWorkspaceAPIKey(context.Background(), data)
	require.NoError(t, err)
	assert.Equal(t, apiKeyID.String(), result.ID)
	assert.Equal(t, "generated-secret", result.Key)
	assert.Equal(t, workspaceID.String(), result.WorkspaceID)
	assert.Equal(t, createdByID.String(), result.CreatedBy)
	assert.Equal(t, []string{"workspaces:read", "workspaces:files:read"}, result.Scopes)
	assert.Equal(t, createdAt.Format(time.RFC3339), result.CreatedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAPIKeyService_CreateWorkspaceAPIKey_RollsBackOnScopeError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	repository := &apiKeyTestRepository{db: db, queries: sqlc.New(db)}
	svc := NewAPIKeyService(zap.NewNop().Sugar(), repository)

	workspaceID := uuid.New()
	createdByID := uuid.New()
	apiKeyID := uuid.New()
	createdAt := time.Date(2026, 6, 4, 10, 11, 12, 0, time.UTC)

	data, err := apikeydata.NewWorkspaceAPIKeyData(
		"workspace key",
		"description",
		"generated-secret",
		workspaceID,
		createdByID,
		[]string{"workspaces:read"},
	)
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO api_keys (created_by_user_id, name, key_hash, description, status)")).
		WithArgs(createdByID, "workspace key", sqlmock.AnyArg(), sql.NullString{String: "description", Valid: true}, "active").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_by_user_id", "created_at", "name", "key_hash", "description", "status", "last_used_at", "revoked_at", "revoked_by_user_id"}).
			AddRow(apiKeyID, createdByID, createdAt, "workspace key", "hashed-key", "description", "active", nil, nil, nil))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO api_key_scopes (api_key_id, scope) VALUES ($1,$2)")).
		WithArgs(apiKeyID, "workspaces:read").
		WillReturnError(assert.AnError)
	mock.ExpectRollback()

	_, err = svc.CreateWorkspaceAPIKey(context.Background(), data)
	require.Error(t, err)
	assert.ErrorContains(t, err, "create api key scope")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAPIKeyService_ListWorkspaceAPIKeys_HappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	repository := &apiKeyTestRepository{db: db, queries: sqlc.New(db)}
	svc := NewAPIKeyService(zap.NewNop().Sugar(), repository)

	workspaceID := uuid.New()
	createdByID := uuid.New()
	apiKeyID := uuid.New()
	createdAt := time.Date(2026, 6, 4, 10, 11, 12, 0, time.UTC)
	lastUsedAt := time.Date(2026, 6, 4, 11, 12, 13, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT\n\tak.id,")).
		WithArgs(workspaceID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_by_user_id", "created_at", "name", "description", "status", "last_used_at"}).
			AddRow(apiKeyID, createdByID, createdAt, "workspace key", "description", "active", lastUsedAt))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT scope\nFROM api_key_scopes\nWHERE api_key_id = $1\nORDER BY scope")).
		WithArgs(apiKeyID).
		WillReturnRows(sqlmock.NewRows([]string{"scope"}).
			AddRow("workspaces:files:read").
			AddRow("workspaces:read"))

	results, err := svc.ListWorkspaceAPIKeys(context.Background(), workspaceID)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, apiKeyID.String(), results[0].ID)
	assert.Equal(t, []string{"workspaces:files:read", "workspaces:read"}, results[0].Scopes)
	assert.Equal(t, createdAt.Format(time.RFC3339), results[0].CreatedAt)
	if assert.NotNil(t, results[0].LastUsedAt) {
		assert.Equal(t, lastUsedAt.Format(time.RFC3339), *results[0].LastUsedAt)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAPIKeyService_DeleteWorkspaceAPIKey_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	repository := &apiKeyTestRepository{db: db, queries: sqlc.New(db)}
	svc := NewAPIKeyService(zap.NewNop().Sugar(), repository)

	workspaceID := uuid.New()
	apiKeyID := uuid.New()
	revokedByID := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta("UPDATE api_keys ak")).
		WithArgs(apiKeyID, workspaceID, uuid.NullUUID{UUID: revokedByID, Valid: true}).
		WillReturnError(sql.ErrNoRows)

	err = svc.DeleteWorkspaceAPIKey(context.Background(), workspaceID, apiKeyID, revokedByID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrWorkspaceAPIKeyNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}
