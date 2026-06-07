package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetWorkspaceAPIKeyParameters_HappyPath(t *testing.T) {
	req := httptest.NewRequest("POST", "/api_keys/workspace/create", strings.NewReader(`{"name":"workspace key","description":"desc","scopes":["workspaces:read","workspaces:files:read"]}`))

	params, err := getWorkspaceAPIKeyParameters(req)
	require.NoError(t, err)
	assert.Equal(t, "workspace key", params.Name)
	assert.Equal(t, "desc", params.Description)
	assert.Equal(t, []string{"workspaces:read", "workspaces:files:read"}, params.Scopes)
}

func TestGetWorkspaceAPIKeyParameters_RejectsUnknownFields(t *testing.T) {
	req := httptest.NewRequest("POST", "/api_keys/workspace/create", strings.NewReader(`{"name":"workspace key","description":"desc","scopes":["workspaces:read"],"users_authorized":["123"]}`))

	_, err := getWorkspaceAPIKeyParameters(req)
	require.Error(t, err)
	assert.ErrorContains(t, err, "unknown field")
}
