package apikeydata

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWorkspaceAPIKeyData_HappyPath(t *testing.T) {
	workspaceID := uuid.New()
	createdByID := uuid.New()

	data, err := NewWorkspaceAPIKeyData(
		" workspace key ",
		" description ",
		" generated-secret ",
		workspaceID,
		createdByID,
		[]string{"workspaces:read", "workspaces:files:read", "workspaces:read"},
	)
	require.NoError(t, err)
	assert.Equal(t, workspaceID, data.WorkspaceID)
	assert.Equal(t, createdByID, data.CreatedByUserID)
	assert.Equal(t, "workspace key", data.Name)
	assert.Equal(t, "description", data.Description)
	assert.Equal(t, "generated-secret", data.Key)
	assert.Equal(t, []string{"workspaces:read", "workspaces:files:read"}, data.Scopes)
}

func TestNewWorkspaceAPIKeyData_RejectsInvalidScope(t *testing.T) {
	_, err := NewWorkspaceAPIKeyData(
		"workspace key",
		"description",
		"generated-secret",
		uuid.New(),
		uuid.New(),
		[]string{"private_files:read"},
	)
	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid api key scope")
}

func TestNewWorkspaceAPIKeyData_RequiresScopes(t *testing.T) {
	_, err := NewWorkspaceAPIKeyData(
		"workspace key",
		"description",
		"generated-secret",
		uuid.New(),
		uuid.New(),
		nil,
	)
	require.Error(t, err)
	assert.ErrorContains(t, err, "at least one scope is required")
}

func TestNewPrivateAPIKeyData_HappyPath(t *testing.T) {
	createdByID := uuid.New()

	data, err := NewPrivateAPIKeyData(
		" private key ",
		" description ",
		" generated-secret ",
		createdByID,
		[]string{"private_files:read", "private_files:write", "private_files:read"},
	)
	require.NoError(t, err)
	assert.Equal(t, createdByID, data.CreatedByUserID)
	assert.Equal(t, "private key", data.Name)
	assert.Equal(t, "description", data.Description)
	assert.Equal(t, "generated-secret", data.Key)
	assert.Equal(t, []string{"private_files:read", "private_files:write"}, data.Scopes)
}

func TestNewPrivateAPIKeyData_RejectsInvalidScope(t *testing.T) {
	_, err := NewPrivateAPIKeyData(
		"private key",
		"description",
		"generated-secret",
		uuid.New(),
		[]string{"workspaces:read"},
	)
	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid api key scope")
}

func TestNewPrivateAPIKeyData_RequiresScopes(t *testing.T) {
	_, err := NewPrivateAPIKeyData(
		"private key",
		"description",
		"generated-secret",
		uuid.New(),
		nil,
	)
	require.Error(t, err)
	assert.ErrorContains(t, err, "at least one scope is required")
}
