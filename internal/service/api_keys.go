package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/apikeydata"
	"github.com/tscrond/fluxsend-backend/internal/repo"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

var ErrWorkspaceAPIKeyNotFound = errors.New("workspace api key not found")

type APIKeyService interface {
	CreateWorkspaceAPIKey(ctx context.Context, akd *apikeydata.APIKeyData) (*CreateAPIKeyResult, error)
	DeleteWorkspaceAPIKey(ctx context.Context, workspaceID, apiKeyID, revokedBy uuid.UUID) error
	ListWorkspaceAPIKeys(ctx context.Context, workspaceID uuid.UUID) ([]APIKeyResult, error)
	CreatePrivateAPIKey(ctx context.Context, akd *apikeydata.APIKeyData) (*CreateAPIKeyResult, error)
	DeletePrivateAPIKey(ctx context.Context, userID, apiKeyID uuid.UUID) error
	ListPrivateAPIKeys(ctx context.Context, userID uuid.UUID) ([]APIKeyResult, error)
}

type apiKeyService struct {
	log        *zap.SugaredLogger
	repository repo.Repository
}

func NewAPIKeyService(log *zap.SugaredLogger, repository repo.Repository) APIKeyService {
	return &apiKeyService{
		log:        log,
		repository: repository,
	}
}

type CreateAPIKeyResult struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Key         string   `json:"key"`
	Scopes      []string `json:"scopes"`
	WorkspaceID string   `json:"workspace_id"`
	UserID      string   `json:"user_id"`
	CreatedBy   string   `json:"created_by"`
	CreatedAt   string   `json:"created_at"`
}

type APIKeyResult struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Scopes      []string `json:"scopes"`
	CreatedBy   string   `json:"created_by"`
	CreatedAt   string   `json:"created_at"`
	LastUsedAt  *string  `json:"last_used_at"`
}

func (s *apiKeyService) CreateWorkspaceAPIKey(ctx context.Context, akd *apikeydata.APIKeyData) (*CreateAPIKeyResult, error) {
	if akd == nil {
		return nil, errors.New("api key data is required")
	}

	keyHash, err := bcrypt.GenerateFromPassword([]byte(akd.Key), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hashing api key: %w", err)
	}

	tx, err := s.repository.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin api key transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	queries := s.repository.Queries().WithTx(tx)

	result, err := queries.CreateAPIKey(ctx, sqlc.CreateAPIKeyParams{
		CreatedByUserID: akd.CreatedByUserID,
		Name:            akd.Name,
		KeyHash:         string(keyHash),
		Description:     sql.NullString{String: akd.Description, Valid: akd.Description != ""},
		Status:          "active",
	})
	if err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}

	for _, scope := range akd.Scopes {
		if err := queries.CreateAPIKeyScope(ctx, sqlc.CreateAPIKeyScopeParams{
			ApiKeyID: result.ID,
			Scope:    scope,
		}); err != nil {
			return nil, fmt.Errorf("create api key scope %q: %w", scope, err)
		}
	}

	if _, err := queries.AssignAPIKeyToWorkspace(ctx, sqlc.AssignAPIKeyToWorkspaceParams{
		ApiKeyID:    result.ID,
		WorkspaceID: akd.WorkspaceID,
	}); err != nil {
		return nil, fmt.Errorf("assign api key to workspace: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit api key transaction: %w", err)
	}
	committed = true

	return NewCreateAPIKeyResult(result, akd.Key, &akd.WorkspaceID, nil, akd.Scopes), nil
}

func NewCreateAPIKeyResult(result sqlc.ApiKey, rawKey string, workspaceID *uuid.UUID, userID *uuid.UUID, scopes []string) *CreateAPIKeyResult {
	if workspaceID == nil && userID == nil {
		log.Println("function expects workspaceID or userID")
		return nil
	}
	responseScopes := make([]string, len(scopes))
	copy(responseScopes, scopes)

	createAPIKeyResult := &CreateAPIKeyResult{
		ID:          result.ID.String(),
		Name:        result.Name,
		Description: result.Description.String,
		Key:         rawKey,
		Scopes:      responseScopes,
		CreatedBy:   result.CreatedByUserID.String(),
		CreatedAt:   result.CreatedAt.UTC().Format(time.RFC3339),
	}

	if workspaceID != nil {
		createAPIKeyResult.WorkspaceID = workspaceID.String()
	}
	if userID != nil {
		createAPIKeyResult.UserID = userID.String()
	}

	return createAPIKeyResult
}

func (s *apiKeyService) DeleteWorkspaceAPIKey(ctx context.Context, workspaceID, apiKeyID, revokedBy uuid.UUID) error {
	_, err := s.repository.Queries().RevokeWorkspaceAPIKey(ctx, sqlc.RevokeWorkspaceAPIKeyParams{
		ID:              apiKeyID,
		WorkspaceID:     workspaceID,
		RevokedByUserID: uuid.NullUUID{UUID: revokedBy, Valid: true},
	})
	if errors.Is(err, sql.ErrNoRows) {
		return ErrWorkspaceAPIKeyNotFound
	}
	if err != nil {
		return fmt.Errorf("revoke workspace api key: %w", err)
	}
	return nil
}

func (s *apiKeyService) ListWorkspaceAPIKeys(ctx context.Context, workspaceID uuid.UUID) ([]APIKeyResult, error) {
	rows, err := s.repository.Queries().ListWorkspaceAPIKeys(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list workspace api keys: %w", err)
	}

	results := make([]APIKeyResult, 0, len(rows))
	for _, row := range rows {
		scopes, err := s.repository.Queries().ListAPIKeyScopes(ctx, row.ID)
		if err != nil {
			return nil, fmt.Errorf("list api key scopes for %s: %w", row.ID, err)
		}

		var lastUsedAt *string
		if row.LastUsedAt.Valid {
			formatted := row.LastUsedAt.Time.UTC().Format(time.RFC3339)
			lastUsedAt = &formatted
		}

		results = append(results, APIKeyResult{
			ID:          row.ID.String(),
			Name:        row.Name,
			Description: row.Description.String,
			Status:      row.Status,
			Scopes:      scopes,
			CreatedBy:   row.CreatedByUserID.String(),
			CreatedAt:   row.CreatedAt.UTC().Format(time.RFC3339),
			LastUsedAt:  lastUsedAt,
		})
	}

	return results, nil
}

func (s *apiKeyService) CreatePrivateAPIKey(ctx context.Context, akd *apikeydata.APIKeyData) (*CreateAPIKeyResult, error) {
	if akd == nil {
		return nil, errors.New("api key data is required")
	}
	keyHash, err := bcrypt.GenerateFromPassword([]byte(akd.Key), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hashing api key: %w", err)
	}

	tx, err := s.repository.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin api key transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	queries := s.repository.Queries().WithTx(tx)

	result, err := queries.CreateAPIKey(ctx, sqlc.CreateAPIKeyParams{
		CreatedByUserID: akd.CreatedByUserID,
		Name:            akd.Name,
		KeyHash:         string(keyHash),
		Description:     sql.NullString{String: akd.Description, Valid: akd.Description != ""},
		Status:          "active",
	})
	if err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}

	for _, scope := range akd.Scopes {
		if err := queries.CreateAPIKeyScope(ctx, sqlc.CreateAPIKeyScopeParams{
			ApiKeyID: result.ID,
			Scope:    scope,
		}); err != nil {
			return nil, fmt.Errorf("create api key scope %q: %w", scope, err)
		}
	}

	assignment, err := queries.AssignAPIKeyToPrivate(ctx, sqlc.AssignAPIKeyToPrivateParams{
		ApiKeyID: result.ID,
		UserID:   akd.CreatedByUserID,
	})
	if err != nil {
		return nil, fmt.Errorf("assign api key to private user: %w", err)
	}
	s.log.Infow("api_key_id_assigned", "api_key_id", assignment.ApiKeyID, "assigned_to_user", assignment.UserID, "api_key_name", result.Name)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit api key transaction: %w", err)
	}
	committed = true

	return NewCreateAPIKeyResult(result, akd.Key, nil, &akd.CreatedByUserID, akd.Scopes), nil
}

func (s *apiKeyService) ListPrivateAPIKeys(ctx context.Context, userID uuid.UUID) ([]APIKeyResult, error) {
	rows, err := s.repository.Queries().ListPrivateAPIKeysByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list private api keys: %w", err)
	}

	results := make([]APIKeyResult, 0, len(rows))
	for _, row := range rows {
		scopes, err := s.repository.Queries().ListAPIKeyScopes(ctx, row.ID)
		if err != nil {
			return nil, fmt.Errorf("list api key scopes for %s: %w", row.ID, err)
		}

		var lastUsedAt *string
		if row.LastUsedAt.Valid {
			formatted := row.LastUsedAt.Time.UTC().Format(time.RFC3339)
			lastUsedAt = &formatted
		}

		results = append(results, APIKeyResult{
			ID:          row.ID.String(),
			Name:        row.Name,
			Description: row.Description.String,
			Status:      row.Status,
			Scopes:      scopes,
			CreatedBy:   row.CreatedByUserID.String(),
			CreatedAt:   row.CreatedAt.UTC().Format(time.RFC3339),
			LastUsedAt:  lastUsedAt,
		})
	}

	return results, nil
}

func (s *apiKeyService) DeletePrivateAPIKey(ctx context.Context, userID, apiKeyID uuid.UUID) error {
	_, err := s.repository.Queries().RevokePrivateAPIKey(ctx, sqlc.RevokePrivateAPIKeyParams{
		ID:              apiKeyID,
		UserID:          userID,
		RevokedByUserID: uuid.NullUUID{UUID: userID, Valid: true},
	})
	if errors.Is(err, sql.ErrNoRows) {
		return ErrWorkspaceAPIKeyNotFound
	}
	if err != nil {
		return fmt.Errorf("revoke private api key: %w", err)
	}
	return nil
}
