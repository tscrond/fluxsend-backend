package apikeydata

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

var allowedWorkspaceScopes = map[string]struct{}{
	"workspaces:read":           {},
	"workspaces:write":          {},
	"workspaces:delete":         {},
	"workspaces:members:read":   {},
	"workspaces:members:manage": {},
	"workspaces:invites:manage": {},
	"workspaces:files:read":     {},
	"workspaces:files:write":    {},
	"workspaces:files:delete":   {},
}

var allowedPrivateScopes = map[string]struct{}{
	"private_files:read":   {},
	"private_files:write":  {},
	"private_files:delete": {},
	"private_files:share":  {},
}

type APIKeyData struct {
	CreatedByUserID uuid.UUID
	WorkspaceID     uuid.UUID
	Name            string
	Description     string
	Key             string
	Scopes          []string
}

func NewWorkspaceAPIKeyData(name, description, key string, workspaceID, createdByID uuid.UUID, scopes []string) (*APIKeyData, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	key = strings.TrimSpace(key)

	if createdByID == uuid.Nil {
		return nil, fmt.Errorf("created by user id is required")
	}
	if workspaceID == uuid.Nil {
		return nil, fmt.Errorf("workspace id is required")
	}
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if key == "" {
		return nil, fmt.Errorf("key is required")
	}

	normalizedScopes, err := normalizeWorkspaceScopes(scopes)
	if err != nil {
		return nil, err
	}

	return &APIKeyData{
		CreatedByUserID: createdByID,
		WorkspaceID:     workspaceID,
		Name:            name,
		Description:     description,
		Key:             key,
		Scopes:          normalizedScopes,
	}, nil
}

func NewPrivateAPIKeyData(name, description, key string, createdByID uuid.UUID, scopes []string) (*APIKeyData, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	key = strings.TrimSpace(key)

	if createdByID == uuid.Nil {
		return nil, fmt.Errorf("created by user id is required")
	}
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if key == "" {
		return nil, fmt.Errorf("key is required")
	}

	normalizedScopes, err := normalizeWorkspaceScopes(scopes)
	if err != nil {
		return nil, err
	}

	return &APIKeyData{
		CreatedByUserID: createdByID,
		Name:            name,
		Description:     description,
		Key:             key,
		Scopes:          normalizedScopes,
	}, nil
}

func normalizeWorkspaceScopes(scopes []string) ([]string, error) {
	if len(scopes) == 0 {
		return nil, fmt.Errorf("at least one scope is required")
	}

	normalized := make([]string, 0, len(scopes))
	seen := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			return nil, fmt.Errorf("scope cannot be empty")
		}
		if _, ok := allowedWorkspaceScopes[scope]; !ok {
			return nil, fmt.Errorf("invalid workspace scope: %s", scope)
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		normalized = append(normalized, scope)
	}

	return normalized, nil
}
