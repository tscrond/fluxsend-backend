package api

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	mailtypes "github.com/tscrond/fluxsend-backend/internal/mailservice/types"
	"github.com/tscrond/fluxsend-backend/internal/mocks"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"github.com/tscrond/fluxsend-backend/internal/service"
)

func newPasswordResetAuthTestServer(t *testing.T, ctrl *gomock.Controller) (*APIServer, sqlmock.Sqlmock, *mocks.MockEmailSender, func()) {
	t.Helper()

	srv, deps := newTestServer(ctrl)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	queries := sqlc.New(db)
	emailSender := mocks.NewMockEmailSender(ctrl)

	deps.repo.EXPECT().Queries().Return(queries).AnyTimes()
	srv.backendConfig.FrontendEndpoint = "https://app.example.com"
	srv.passwordAuth = service.NewPasswordAuthService(zap.NewNop().Sugar(), emailSender, queries, deps.repo, "noreply@example.com")

	cleanup := func() {
		mock.ExpectClose()
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	}

	return srv, mock, emailSender, cleanup
}

func newIdentityRows(userID uuid.UUID, providers ...string) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id", "user_id", "provider", "provider_user_id", "email", "email_verified", "name", "avatar_url", "created_at"})
	for _, provider := range providers {
		rows.AddRow(
			uuid.New(),
			userID,
			provider,
			provider+"-user",
			sql.NullString{},
			sql.NullBool{},
			sql.NullString{},
			sql.NullString{},
			sql.NullTime{},
		)
	}
	return rows
}

func newAuthRateLimitRows(key, scope string, blockedUntil sql.NullTime) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "key", "scope", "attempt_count", "blocked_until", "created_at", "updated_at"}).
		AddRow(uuid.New(), key, scope, 0, blockedUntil, sql.NullTime{}, sql.NullTime{})
}

func newEmailVerificationChallengeRows(challenge sqlc.EmailVerificationChallenge) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "email", "user_id", "purpose", "code_hash", "expires_at", "max_attempts", "resend_available_at", "requested_by_ip", "request_context", "consumed_at", "created_at"}).
		AddRow(
			challenge.ID,
			challenge.Email,
			challenge.UserID,
			challenge.Purpose,
			challenge.CodeHash,
			challenge.ExpiresAt,
			challenge.MaxAttempts,
			challenge.ResendAvailableAt,
			challenge.RequestedByIp,
			challenge.RequestContext,
			challenge.ConsumedAt,
			challenge.CreatedAt,
		)
}

func newUserRows(userID uuid.UUID, email string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "user_bucket", "user_email", "created_at", "plan_id"}).
		AddRow(userID, sql.NullString{}, email, time.Now(), uuid.NullUUID{})
}

func TestCreateSelfPasswordResetRequestHandler_SendsLinkToDeliveryAddress(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, mock, emailSender, cleanup := newPasswordResetAuthTestServer(t, ctrl)
	defer cleanup()

	userID := uuid.New()
	challengeID := uuid.New()
	accountEmail := "account@example.com"
	deliveryEmail := "delivery@example.com"
	userCooldownKey := passwordResetSelfUserCooldownKey(userID)
	ipCooldownKey := passwordResetSelfIPCooldownKey("unknown")
	cooldownUntil := time.Now().Add(passwordResetRequestCooldown)

	mock.ExpectQuery("").
		WithArgs(userID).
		WillReturnRows(newIdentityRows(userID, "password"))
	mock.ExpectQuery("").
		WithArgs(userCooldownKey, authRateLimitScopeResetSelfInit).
		WillReturnRows(sqlmock.NewRows([]string{"id", "key", "scope", "attempt_count", "blocked_until", "created_at", "updated_at"}))
	mock.ExpectQuery("").
		WithArgs(userCooldownKey, authRateLimitScopeResetSelfInit, 0).
		WillReturnRows(newAuthRateLimitRows(userCooldownKey, authRateLimitScopeResetSelfInit, sql.NullTime{}))
	mock.ExpectQuery("").
		WithArgs(ipCooldownKey, authRateLimitScopeResetSelfInit).
		WillReturnRows(sqlmock.NewRows([]string{"id", "key", "scope", "attempt_count", "blocked_until", "created_at", "updated_at"}))
	mock.ExpectQuery("").
		WithArgs(ipCooldownKey, authRateLimitScopeResetSelfInit, 0).
		WillReturnRows(newAuthRateLimitRows(ipCooldownKey, authRateLimitScopeResetSelfInit, sql.NullTime{}))
	mock.ExpectExec("").
		WithArgs(uuid.NullUUID{UUID: userID, Valid: true}, passwordResetSelfChallengePurpose).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("").
		WithArgs(
			deliveryEmail,
			uuid.NullUUID{UUID: userID, Valid: true},
			passwordResetSelfChallengePurpose,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			"unknown",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnRows(newEmailVerificationChallengeRows(sqlc.EmailVerificationChallenge{
			ID:                challengeID,
			Email:             deliveryEmail,
			UserID:            uuid.NullUUID{UUID: userID, Valid: true},
			Purpose:           passwordResetSelfChallengePurpose,
			CodeHash:          hashEmailVerificationToken("generated-code"),
			ExpiresAt:         time.Now().Add(passwordResetChallengeTTL),
			ResendAvailableAt: cooldownUntil,
			RequestedByIp:     "unknown",
			RequestContext:    []byte(`{}`),
			ConsumedAt:        sql.NullTime{},
			CreatedAt:         sql.NullTime{},
		}))
	mock.ExpectQuery("").
		WithArgs(userCooldownKey, authRateLimitScopeResetSelfInit, sqlmock.AnyArg()).
		WillReturnRows(newAuthRateLimitRows(userCooldownKey, authRateLimitScopeResetSelfInit, sql.NullTime{Time: cooldownUntil, Valid: true}))
	mock.ExpectQuery("").
		WithArgs(ipCooldownKey, authRateLimitScopeResetSelfInit, sqlmock.AnyArg()).
		WillReturnRows(newAuthRateLimitRows(ipCooldownKey, authRateLimitScopeResetSelfInit, sql.NullTime{Time: cooldownUntil, Valid: true}))
	emailSender.EXPECT().Send(gomock.Any()).DoAndReturn(func(message mailtypes.MessageConfig) (any, error) {
		assert.Equal(t, []string{deliveryEmail}, message.To)
		assert.Contains(t, message.Body, "/password/reset/verify/"+challengeID.String())
		assert.Contains(t, message.Body, "email=account%40example.com")
		assert.NotContains(t, message.Body, "email=delivery%40example.com")
		assert.Contains(t, message.Body, "flow=self")
		return "ok", nil
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/password/reset/self", bytes.NewBufferString(`{"email":"delivery@example.com"}`))
	req = injectAuth(req, accountEmail, userID.String(), defaultPlan(uuid.New().String()))
	w := httptest.NewRecorder()

	srv.createSelfPasswordResetRequestHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"sent":true`)
}

func TestCreateSelfPasswordResetRequestHandler_RequiresPasswordIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, mock, _, cleanup := newPasswordResetAuthTestServer(t, ctrl)
	defer cleanup()

	userID := uuid.New()
	mock.ExpectQuery("").
		WithArgs(userID).
		WillReturnRows(newIdentityRows(userID, "github"))

	req := httptest.NewRequest(http.MethodPost, "/auth/password/reset/self", bytes.NewBufferString(`{"email":"delivery@example.com"}`))
	req = injectAuth(req, "broken-email", userID.String(), defaultPlan(uuid.New().String()))
	w := httptest.NewRecorder()

	srv.createSelfPasswordResetRequestHandler(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "password_reset_not_available")
	assert.Contains(t, w.Body.String(), "Password reset is not available for this account.")
}

func TestCreateSelfPasswordResetRequestHandler_BlocksActiveCooldown(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, mock, _, cleanup := newPasswordResetAuthTestServer(t, ctrl)
	defer cleanup()

	userID := uuid.New()
	userCooldownKey := passwordResetSelfUserCooldownKey(userID)

	mock.ExpectQuery("").
		WithArgs(userID).
		WillReturnRows(newIdentityRows(userID, "password"))
	mock.ExpectQuery("").
		WithArgs(userCooldownKey, authRateLimitScopeResetSelfInit).
		WillReturnRows(newAuthRateLimitRows(userCooldownKey, authRateLimitScopeResetSelfInit, sql.NullTime{Time: time.Now().Add(time.Minute), Valid: true}))

	req := httptest.NewRequest(http.MethodPost, "/auth/password/reset/self", bytes.NewBufferString(`{"email":"delivery@example.com"}`))
	req = injectAuth(req, "broken-email", userID.String(), defaultPlan(uuid.New().String()))
	w := httptest.NewRecorder()

	srv.createSelfPasswordResetRequestHandler(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), "password_reset_cooldown")
	assert.Contains(t, w.Body.String(), "Please wait before requesting another password reset link.")
}

func TestVerifyPasswordResetRequestHandler_AllowsSelfResetWithDeliveryEmail(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, mock, _, cleanup := newPasswordResetAuthTestServer(t, ctrl)
	defer cleanup()

	challengeID := uuid.New()
	userID := uuid.New()
	accountEmail := "account@example.com"
	code := "reset-code"
	deliveryEmail := "delivery@example.com"

	mock.ExpectQuery("").
		WithArgs(challengeID).
		WillReturnRows(newEmailVerificationChallengeRows(sqlc.EmailVerificationChallenge{
			ID:                challengeID,
			Email:             deliveryEmail,
			UserID:            uuid.NullUUID{UUID: userID, Valid: true},
			Purpose:           passwordResetSelfChallengePurpose,
			CodeHash:          hashEmailVerificationToken(code),
			ExpiresAt:         time.Now().Add(time.Minute),
			ResendAvailableAt: time.Now().Add(time.Minute),
			RequestedByIp:     "unknown",
			RequestContext:    []byte(`{}`),
			ConsumedAt:        sql.NullTime{},
			CreatedAt:         sql.NullTime{},
		}))
	mock.ExpectQuery("").
		WithArgs(userID).
		WillReturnRows(newIdentityRows(userID, "password"))
	mock.ExpectQuery("").
		WithArgs(userID).
		WillReturnRows(newUserRows(userID, accountEmail))
	mock.ExpectQuery("").
		WithArgs("challenge:"+challengeID.String(), authRateLimitScopePasswordReset).
		WillReturnRows(sqlmock.NewRows([]string{"id", "key", "scope", "attempt_count", "blocked_until", "created_at", "updated_at"}))
	mock.ExpectQuery("").
		WithArgs("challenge:"+challengeID.String(), authRateLimitScopePasswordReset, 0).
		WillReturnRows(newAuthRateLimitRows("challenge:"+challengeID.String(), authRateLimitScopePasswordReset, sql.NullTime{}))

	req := httptest.NewRequest(http.MethodPost, "/auth/password/reset/verify/"+challengeID.String(), bytes.NewBufferString(`{"email":"account@example.com","code":"reset-code"}`))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, func() *chi.Context {
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", challengeID.String())
		return rctx
	}()))
	w := httptest.NewRecorder()

	srv.verifyPasswordResetRequestHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"verified":true`)
	assert.NotContains(t, strings.ToLower(w.Body.String()), "verification_failed")
}
