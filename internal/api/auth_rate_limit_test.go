package api

import (
	"bytes"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestPasswordLoginHandlerBlocksRateLimitedIP(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, mock, _, cleanup := newPasswordResetAuthTestServer(t, ctrl)
	defer cleanup()

	key := passwordLoginRateLimitKey("unknown")
	mock.ExpectQuery("INSERT INTO auth_rate_limits").
		WithArgs(key, authRateLimitScopeLoginPassword, sqlmock.AnyArg(), passwordLoginMaxAttempts, sqlmock.AnyArg()).
		WillReturnRows(authRateLimitRow(key, authRateLimitScopeLoginPassword, time.Now().Add(time.Minute)))

	req := httptest.NewRequest(http.MethodPost, "/auth/password/login", bytes.NewBufferString(`{"email":"user@example.com","password":"password"}`))
	w := httptest.NewRecorder()

	srv.passwordLoginHandler(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.NotEmpty(t, w.Header().Get("Retry-After"))
	assert.Contains(t, w.Body.String(), "Too many sign-in attempts")
}

func TestCreateRegistrationRequestBlocksRateLimitedIP(t *testing.T) {
	ctrl := gomock.NewController(t)
	srv, mock, _, cleanup := newPasswordResetAuthTestServer(t, ctrl)
	defer cleanup()

	key := passwordRegistrationRateLimitKey("unknown")
	mock.ExpectQuery("INSERT INTO auth_rate_limits").
		WithArgs(key, authRateLimitScopeRegisterSend, sqlmock.AnyArg(), passwordRegistrationMaxAttempts, sqlmock.AnyArg()).
		WillReturnRows(authRateLimitRow(key, authRateLimitScopeRegisterSend, time.Now().Add(time.Minute)))

	req := httptest.NewRequest(http.MethodPost, "/auth/password/register", bytes.NewBufferString(`{"email":"user@example.com","password":"password"}`))
	w := httptest.NewRecorder()

	srv.createRegistrationRequest(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.NotEmpty(t, w.Header().Get("Retry-After"))
	assert.Contains(t, w.Body.String(), "Too many registration attempts")
}

func authRateLimitRow(key, scope string, blockedUntil time.Time) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "key", "scope", "attempt_count", "blocked_until", "created_at", "updated_at"}).
		AddRow(uuid.New(), key, scope, passwordLoginMaxAttempts+1, sql.NullTime{Time: blockedUntil, Valid: true}, sql.NullTime{}, sql.NullTime{})
}
