package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/tscrond/fluxsend-backend/internal/apimocks"
	storagetypes "github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	"github.com/tscrond/fluxsend-backend/internal/config"
	"github.com/tscrond/fluxsend-backend/internal/mocks"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"github.com/tscrond/fluxsend-backend/internal/userdata"
)

// ── helpers ───────────────────────────────────────────────────────────────────

type testDeps struct {
	files      *apimocks.MockFileService
	shares     *apimocks.MockShareService
	workspaces *apimocks.MockWorkspaceService
	users      *apimocks.MockUserService
	wsFiles    *apimocks.MockWorkspaceFileService
	repo       *mocks.MockRepository
	stor       *mocks.MockObjectStorage
}

func newTestServer(ctrl *gomock.Controller) (*APIServer, *testDeps) {
	deps := &testDeps{
		files:      apimocks.NewMockFileService(ctrl),
		shares:     apimocks.NewMockShareService(ctrl),
		workspaces: apimocks.NewMockWorkspaceService(ctrl),
		users:      apimocks.NewMockUserService(ctrl),
		wsFiles:    apimocks.NewMockWorkspaceFileService(ctrl),
		repo:       mocks.NewMockRepository(ctrl),
		stor:       mocks.NewMockObjectStorage(ctrl),
	}

	srv := NewAPIServer(
		zap.NewNop().Sugar(),
		config.BackendConfig{},
		nil, // email sender — not needed for these tests
		deps.stor,
		nil, // CloudFrontSigner — not needed
		deps.repo,
		nil, // authProviders — not needed
		nil, // tokenEncryptor — not needed
		deps.files,
		deps.shares,
		deps.users,
		deps.workspaces,
		deps.wsFiles,
	)
	return srv, deps
}

// injectAuth returns an *http.Request with an AuthorizedUserWithPlan in its context.
func injectAuth(r *http.Request, email, internalID string, plan userdata.UserPlan) *http.Request {
	uwp := &userdata.AuthorizedUserWithPlan{
		AuthorizedUserInfo: userdata.AuthorizedUserInfo{
			InternalID: internalID,
			Email:      email,
			Name:       "Test User",
		},
		UserPlan: plan,
	}
	return r.WithContext(context.WithValue(r.Context(), userdata.AuthorizedUserWithPlanContextKey, uwp))
}

// defaultPlan returns an unlimited plan suitable for most happy-path tests.
func defaultPlan(planID string) userdata.UserPlan {
	return userdata.UserPlan{
		PlanID:             planID,
		PlanName:           "unlimited",
		MaxFileSizeBytes:   0, // 0 = no limit
		MaxFiles:           0,
		MaxFilesSentPerDay: 0,
		MaxSharesPerDay:    0,
	}
}

// ── uploadHandler tests ───────────────────────────────────────────────────────

func TestUploadHandler_WrongMethod(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, _ := newTestServer(ctrl)

	req := httptest.NewRequest(http.MethodGet, "/files/upload", nil)
	w := httptest.NewRecorder()
	srv.uploadHandler(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUploadHandler_NoAuth(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, _ := newTestServer(ctrl)

	req := httptest.NewRequest(http.MethodPost, "/files/upload", nil)
	w := httptest.NewRecorder()
	srv.uploadHandler(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestUploadHandler_FileTooLarge(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, _ := newTestServer(ctrl)

	userID := uuid.New().String()
	planID := uuid.New().String()
	plan := defaultPlan(planID)
	plan.MaxFileSizeBytes = 100 // very small

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "bigfile.bin")
	fw.Write(bytes.Repeat([]byte("x"), 200)) // larger than 100 bytes
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/files/upload", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = injectAuth(req, "test@example.com", userID, plan)

	w := httptest.NewRecorder()
	srv.uploadHandler(w, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

func TestUploadHandler_PlanLimitExceeded(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, deps := newTestServer(ctrl)

	userID := uuid.New()
	planID := uuid.New()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// CheckUploadQuota returns files_exceeded=true
	mock.ExpectQuery("CheckUploadQuota|files_exceeded|max_files").
		WillReturnRows(sqlmock.NewRows([]string{"files_exceeded", "storage_exceeded", "daily_exceeded"}).
			AddRow(true, false, false))

	deps.repo.EXPECT().Queries().Return(sqlc.New(db))

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "note.txt")
	fw.Write([]byte("hello"))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/files/upload", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = injectAuth(req, "test@example.com", userID.String(), defaultPlan(planID.String()))

	w := httptest.NewRecorder()
	srv.uploadHandler(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestUploadHandler_FileAlreadyExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, deps := newTestServer(ctrl)

	userID := uuid.New()
	planID := uuid.New()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("").
		WillReturnRows(sqlmock.NewRows([]string{"files_exceeded", "storage_exceeded", "daily_exceeded"}).
			AddRow(false, false, false))
	deps.repo.EXPECT().Queries().Return(sqlc.New(db))
	deps.files.EXPECT().Upload(gomock.Any(), gomock.Any()).Return(storagetypes.ErrFileAlreadyExists)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "existing.txt")
	fw.Write([]byte("data"))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/files/upload", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = injectAuth(req, "test@example.com", userID.String(), defaultPlan(planID.String()))

	w := httptest.NewRecorder()
	srv.uploadHandler(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestUploadHandler_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, deps := newTestServer(ctrl)

	userID := uuid.New()
	planID := uuid.New()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("").
		WillReturnRows(sqlmock.NewRows([]string{"files_exceeded", "storage_exceeded", "daily_exceeded"}).
			AddRow(false, false, false))
	deps.repo.EXPECT().Queries().Return(sqlc.New(db))
	deps.files.EXPECT().Upload(gomock.Any(), gomock.Any()).Return(nil)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "photo.png")
	fw.Write([]byte("png-data"))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/files/upload", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = injectAuth(req, "test@example.com", userID.String(), defaultPlan(planID.String()))

	w := httptest.NewRecorder()
	srv.uploadHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ── shareWith tests ───────────────────────────────────────────────────────────

func TestShareWith_WrongMethod(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, _ := newTestServer(ctrl)

	req := httptest.NewRequest(http.MethodGet, "/files/share", nil)
	w := httptest.NewRecorder()
	srv.shareWith(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestShareWith_NoAuth(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, _ := newTestServer(ctrl)

	req := httptest.NewRequest(http.MethodPost, "/files/share", bytes.NewBufferString(`{}`))
	w := httptest.NewRecorder()
	srv.shareWith(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestShareWith_InvalidJSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, _ := newTestServer(ctrl)

	userID := uuid.New()
	planID := uuid.New()

	req := httptest.NewRequest(http.MethodPost, "/files/share", bytes.NewBufferString("not-json"))
	req = injectAuth(req, "user@example.com", userID.String(), defaultPlan(planID.String()))

	w := httptest.NewRecorder()
	srv.shareWith(w, req)

	// JSON decode fails before plan check
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestShareWith_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, deps := newTestServer(ctrl)

	userID := uuid.New()
	planID := uuid.New()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("").
		WillReturnRows(sqlmock.NewRows([]string{"shares_exceeded"}).AddRow(false))

	deps.repo.EXPECT().Queries().Return(sqlc.New(db))
	deps.shares.EXPECT().ShareWith(gomock.Any(), "sender@example.com", gomock.Any(),
		"recipient@example.com", []string{"file.txt"}, "24h", "", false).
		Return(nil, "", nil)

	body, _ := json.Marshal(map[string]any{
		"email":      "recipient@example.com",
		"objects":    []string{"file.txt"},
		"duration":   "24h",
		"password":   "",
		"send_email": false,
	})

	req := httptest.NewRequest(http.MethodPost, "/files/share", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = injectAuth(req, "sender@example.com", userID.String(), defaultPlan(planID.String()))

	w := httptest.NewRecorder()
	srv.shareWith(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ── getDataSharedForUser tests ────────────────────────────────────────────────

func TestGetDataSharedForUser_WrongMethod(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, _ := newTestServer(ctrl)

	req := httptest.NewRequest(http.MethodPost, "/files/received", nil)
	w := httptest.NewRecorder()
	srv.getDataSharedForUser(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetDataSharedForUser_NoAuth(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, _ := newTestServer(ctrl)

	req := httptest.NewRequest(http.MethodGet, "/files/received", nil)
	w := httptest.NewRecorder()
	srv.getDataSharedForUser(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGetDataSharedForUser_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, deps := newTestServer(ctrl)

	userID := uuid.New()
	planID := uuid.New()

	deps.shares.EXPECT().GetSharedForUser(gomock.Any(), "user@example.com").
		Return(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/files/received", nil)
	req = injectAuth(req, "user@example.com", userID.String(), defaultPlan(planID.String()))

	w := httptest.NewRecorder()
	srv.getDataSharedForUser(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ── getDataSharedByUser tests ─────────────────────────────────────────────────

func TestGetDataSharedByUser_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, deps := newTestServer(ctrl)

	userID := uuid.New()
	planID := uuid.New()

	deps.shares.EXPECT().GetSharedByUser(gomock.Any(), "user@example.com").
		Return(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/files/shared_by_user", nil)
	req = injectAuth(req, "user@example.com", userID.String(), defaultPlan(planID.String()))

	w := httptest.NewRecorder()
	srv.getDataSharedByUser(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ── listWorkspaces tests ──────────────────────────────────────────────────────

func TestListWorkspaces_WrongMethod(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, _ := newTestServer(ctrl)

	req := httptest.NewRequest(http.MethodPost, "/workspaces", nil)
	w := httptest.NewRecorder()
	srv.listWorkspaces(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestListWorkspaces_NoAuth(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, _ := newTestServer(ctrl)

	req := httptest.NewRequest(http.MethodGet, "/workspaces", nil)
	w := httptest.NewRecorder()
	srv.listWorkspaces(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestListWorkspaces_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, deps := newTestServer(ctrl)

	userID := uuid.New()
	planID := uuid.New()

	deps.workspaces.EXPECT().GetUserWorkspaces(gomock.Any(), userID).Return(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/workspaces", nil)
	req = injectAuth(req, "user@example.com", userID.String(), defaultPlan(planID.String()))

	w := httptest.NewRecorder()
	srv.listWorkspaces(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ── createWorkspace tests ─────────────────────────────────────────────────────

func TestCreateWorkspace_WrongMethod(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, _ := newTestServer(ctrl)

	req := httptest.NewRequest(http.MethodGet, "/workspaces", nil)
	w := httptest.NewRecorder()
	srv.createWorkspace(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestCreateWorkspace_NoAuth(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, _ := newTestServer(ctrl)

	req := httptest.NewRequest(http.MethodPost, "/workspaces", nil)
	w := httptest.NewRecorder()
	srv.createWorkspace(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCreateWorkspace_NameTooLong(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, deps := newTestServer(ctrl)

	userID := uuid.New()
	planID := uuid.New()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("").
		WillReturnRows(sqlmock.NewRows([]string{"workspaces_exceeded"}).AddRow(false))

	deps.repo.EXPECT().Queries().Return(sqlc.New(db))

	payload, _ := json.Marshal(map[string]string{
		"name": string(make([]byte, 65)),
		"slug": "valid-slug",
	})
	req := httptest.NewRequest(http.MethodPost, "/workspaces", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	req = injectAuth(req, "user@example.com", userID.String(), defaultPlan(planID.String()))

	w := httptest.NewRecorder()
	srv.createWorkspace(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateWorkspace_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, deps := newTestServer(ctrl)

	userID := uuid.New()
	planID := uuid.New()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("").
		WillReturnRows(sqlmock.NewRows([]string{"workspaces_exceeded"}).AddRow(false))

	deps.repo.EXPECT().Queries().Return(sqlc.New(db))
	deps.workspaces.EXPECT().CreateWorkspace(gomock.Any(), userID, "My Workspace", "my-workspace").Return(nil)

	payload, _ := json.Marshal(map[string]string{
		"name": "My Workspace",
		"slug": "my-workspace",
	})
	req := httptest.NewRequest(http.MethodPost, "/workspaces", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	req = injectAuth(req, "user@example.com", userID.String(), defaultPlan(planID.String()))

	w := httptest.NewRecorder()
	srv.createWorkspace(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
