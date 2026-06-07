package api

import (
	"context"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tscrond/fluxsend-backend/internal/apikeydata"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"github.com/tscrond/fluxsend-backend/internal/service"
	"github.com/tscrond/fluxsend-backend/internal/userdata"
	"go.uber.org/mock/gomock"
)

func TestGetWorkspaceAPIKeyParameters_HappyPath(t *testing.T) {
	req := httptest.NewRequest("POST", "/api_keys/workspace/create", strings.NewReader(`{"name":"workspace key","description":"desc","scopes":["workspaces:read","workspaces:files:read"]}`))

	params, err := getAPIKeyParameters(req)
	require.NoError(t, err)
	assert.Equal(t, "workspace key", params.Name)
	assert.Equal(t, "desc", params.Description)
	assert.Equal(t, []string{"workspaces:read", "workspaces:files:read"}, params.Scopes)
}

func TestGetWorkspaceAPIKeyParameters_RejectsUnknownFields(t *testing.T) {
	req := httptest.NewRequest("POST", "/api_keys/workspace/create", strings.NewReader(`{"name":"workspace key","description":"desc","scopes":["workspaces:read"],"users_authorized":["123"]}`))

	_, err := getAPIKeyParameters(req)
	require.Error(t, err)
	assert.ErrorContains(t, err, "unknown field")
}

func TestCreatePrivateAPIKey_UsesInternalUserID(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, deps := newTestServer(ctrl)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	internalID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT (\n\tSELECT COUNT(*)\n\tFROM api_key_user_assignments a")).
		WithArgs(internalID).
		WillReturnRows(sqlmock.NewRows([]string{"api_keys_exceeded"}).AddRow(false))
	deps.repo.EXPECT().Queries().Return(sqlc.New(db))
	deps.apiKeys.EXPECT().CreatePrivateAPIKey(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, data *apikeydata.APIKeyData) (*service.CreateAPIKeyResult, error) {
			require.NotNil(t, data)
			assert.Equal(t, internalID, data.CreatedByUserID)
			assert.Equal(t, []string{"private_files:read"}, data.Scopes)
			return &service.CreateAPIKeyResult{
				ID:        uuid.NewString(),
				Name:      data.Name,
				Key:       data.Key,
				Scopes:    data.Scopes,
				UserID:    data.CreatedByUserID.String(),
				CreatedBy: data.CreatedByUserID.String(),
				CreatedAt: "2026-06-07T10:00:00Z",
			}, nil
		},
	)

	req := httptest.NewRequest("POST", "/api_keys/private/create", strings.NewReader(`{"name":"private key","description":"desc","scopes":["private_files:read"]}`))
	req = req.WithContext(context.WithValue(req.Context(), userdata.AuthorizedUserWithPlanContextKey, &userdata.AuthorizedUserWithPlan{
		AuthorizedUserInfo: userdata.AuthorizedUserInfo{
			InternalID: internalID.String(),
			Id:         "oauth-subject",
			Email:      "test@example.com",
			Name:       "Test User",
		},
		UserPlan: userdata.UserPlan{PlanID: uuid.NewString(), PlanName: "starter", MaxPrivateAPIKeys: 5},
	}))

	w := httptest.NewRecorder()
	srv.createPrivateAPIKey(w, req)

	assert.Equal(t, 200, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreatePrivateAPIKey_InvalidScopeReturnsBadRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, deps := newTestServer(ctrl)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	internalID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT (\n\tSELECT COUNT(*)\n\tFROM api_key_user_assignments a")).
		WithArgs(internalID).
		WillReturnRows(sqlmock.NewRows([]string{"api_keys_exceeded"}).AddRow(false))
	deps.repo.EXPECT().Queries().Return(sqlc.New(db))

	req := httptest.NewRequest("POST", "/api_keys/private/create", strings.NewReader(`{"name":"private key","description":"desc","scopes":["workspaces:read"]}`))
	req = req.WithContext(context.WithValue(req.Context(), userdata.AuthorizedUserWithPlanContextKey, &userdata.AuthorizedUserWithPlan{
		AuthorizedUserInfo: userdata.AuthorizedUserInfo{
			InternalID: internalID.String(),
			Id:         "oauth-subject",
			Email:      "test@example.com",
			Name:       "Test User",
		},
		UserPlan: userdata.UserPlan{PlanID: uuid.NewString(), PlanName: "starter", MaxPrivateAPIKeys: 5},
	}))

	w := httptest.NewRecorder()
	srv.createPrivateAPIKey(w, req)

	assert.Equal(t, 400, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}
