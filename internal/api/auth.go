package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/tscrond/fluxsend-backend/internal/logger"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"github.com/tscrond/fluxsend-backend/internal/userdata"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
	pkghash "github.com/tscrond/fluxsend-backend/pkg/hash"
)

const (
	IsProd               = true
	sessionCookieName    = "session_id"
	oauthStateCookieName = "oauth_state"
	sessionDuration      = 24 * time.Hour
)

var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

type passwordAttachChallengeContext struct {
	PasswordHash string `json:"password_hash"`
}

func (s *APIServer) authNotEnabledHandler(w http.ResponseWriter, r *http.Request) {
	pkg.WriteJSONResponse(w, http.StatusForbidden, "auth_not_enabled", map[string]any{
		"error": "Authentication is not enabled for this provider.",
	})
}

func (s *APIServer) authCallbackNotEnabledHandler(w http.ResponseWriter, r *http.Request) {
	pkg.WriteJSONResponse(w, http.StatusForbidden, "auth_not_enabled", map[string]any{
		"error": "Authentication is not enabled for this provider.",
	})
}

func (s *APIServer) authProvidersHandler(w http.ResponseWriter, r *http.Request) {
	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"google":   s.authProviders["google"] != nil,
		"github":   s.authProviders["github"] != nil,
		"password": s.authProviders["password"] != nil,
	})
}

func (s *APIServer) passwordLoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.log.Errorw("invalid method for login", "method", r.Method)
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "method_not_allowed", nil)
		return
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.log.Errorw("failed to decode login request", "error", err)
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_request", nil)
		return
	}

	if req.Email == "" || req.Password == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "missing_fields", nil)
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if !emailPattern.MatchString(req.Email) {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_email", nil)
		return
	}

	userID, err := s.passwordAuth.LoginUser(r.Context(), req.Email, req.Password)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "invalid_credentials", nil)
		return
	}

	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		s.log.Errorw("failed to parse user id returned by password login", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	bucketName := pkg.GetUserBucketName(s.bucketHandler.GetBucketBaseName(), parsedUserID.String())
	if err := s.repository.Queries().UpdateUserBucketNameById(r.Context(), sqlc.UpdateUserBucketNameByIdParams{
		UserBucket: sql.NullString{String: bucketName, Valid: true},
		ID:         parsedUserID,
	}); err != nil {
		s.log.Warnw("failed to backfill user bucket name during password login", "user_id", parsedUserID, "error", err)
	}

	if err := s.bucketHandler.CreateBucketIfNotExists(r.Context(), parsedUserID.String()); err != nil {
		s.log.Warnw("failed to create bucket for password user", "user_id", parsedUserID, "error", err)
	}

	sessionID := uuid.New()
	if _, err := s.repository.Queries().CreateSession(r.Context(), sqlc.CreateSessionParams{
		ID:                  sessionID,
		UserID:              parsedUserID,
		Provider:            "password",
		ProviderAccessToken: sql.NullString{Valid: false},
		ExpiresAt:           time.Now().Add(sessionDuration),
	}); err != nil {
		s.log.Errorw("failed to create password session", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "session_error", nil)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID.String(),
		HttpOnly: true,
		Secure:   IsProd,
		Path:     "/",
		Expires:  time.Now().Add(sessionDuration),
		SameSite: http.SameSiteLaxMode,
	})

	pkg.WriteJSONResponse(w, http.StatusOK, "login_success", map[string]any{"user_id": parsedUserID.String()})

}

func (s *APIServer) createPasswordAttachRequestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.log.Errorw("invalid method for password attach request", "method", r.Method)
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "method_not_allowed", nil)
		return
	}

	authUser, userID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "access_denied", nil)
		return
	}

	email := strings.TrimSpace(strings.ToLower(authUser.Email))
	if email == "" || !emailPattern.MatchString(email) {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_email", nil)
		return
	}

	var req struct {
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.log.Errorw("failed to decode password attach request", "error", err)
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_request", nil)
		return
	}

	req.Password = strings.TrimSpace(req.Password)
	if len(req.Password) < 8 {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "weak_password", nil)
		return
	}

	identities, err := s.repository.Queries().GetIdentitiesByUserID(r.Context(), userID)
	if err != nil {
		s.log.Errorw("failed to load identities for password attach", "user_id", userID, "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}
	for _, identity := range identities {
		if identity.Provider == "password" {
			pkg.WriteJSONResponse(w, http.StatusConflict, "password_identity_exists", nil)
			return
		}
	}

	passwordHash, err := pkghash.HashPasswordArgon2id(req.Password)
	if err != nil {
		s.log.Errorw("failed to hash password for attach", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	ctxPayload, err := json.Marshal(passwordAttachChallengeContext{PasswordHash: passwordHash})
	if err != nil {
		s.log.Errorw("failed to marshal attach challenge context", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	token := pkg.GenerateEmailConfirmationCode()
	challenge, err := s.repository.Queries().CreateEmailVerificationChallenge(r.Context(), sqlc.CreateEmailVerificationChallengeParams{
		Email:             email,
		UserID:            uuid.NullUUID{UUID: userID, Valid: true},
		Purpose:           "password_attach",
		CodeHash:          hashEmailVerificationToken(token),
		ExpiresAt:         time.Now().Add(5 * time.Minute),
		RequestedByIp:     pkg.GetClientIPFromContext(r.Context()),
		RequestContext:    ctxPayload,
		ResendAvailableAt: time.Now().Add(60 * time.Second),
	})
	if err != nil {
		s.log.Errorw("failed to create password attach challenge", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	verifyLink := fmt.Sprintf("%s/password/attach/verify/%s?email=%s&code=%s",
		s.backendConfig.FrontendEndpoint,
		url.PathEscape(challenge.ID.String()),
		url.QueryEscape(email),
		url.QueryEscape(token),
	)

	if err := s.passwordAuth.SendConfirmationEmail(r.Context(), email, token, verifyLink); err != nil {
		s.log.Errorw("failed to send password attach verification email", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "password_attach_request_success", map[string]any{"challenge_id": challenge.ID.String()})
}

func (s *APIServer) verifyPasswordAttachRequestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.log.Errorw("invalid method for password attach verification", "method", r.Method)
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "method_not_allowed", nil)
		return
	}

	challengeID := chi.URLParam(r, "id")
	if challengeID == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "missing_fields", nil)
		return
	}

	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.log.Errorw("failed to decode password attach verification request", "error", err)
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_request", nil)
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))
	code := strings.TrimSpace(req.Code)
	if email == "" || code == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "missing_fields", nil)
		return
	}
	if !emailPattern.MatchString(email) {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_email", nil)
		return
	}

	parsedChallengeID, err := uuid.Parse(challengeID)
	if err != nil {
		s.log.Errorw("failed to parse password attach challenge id", "error", err)
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_request", nil)
		return
	}

	challenge, err := s.repository.Queries().GetEmailVerificationChallengeById(r.Context(), parsedChallengeID)
	if err != nil {
		s.log.Errorw("failed to load password attach challenge", "error", err)
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "verification_failed", nil)
		return
	}
	if challenge.Purpose != "password_attach" || !challenge.UserID.Valid {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "verification_failed", nil)
		return
	}
	if strings.TrimSpace(strings.ToLower(challenge.Email)) != email {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "verification_failed", nil)
		return
	}
	if challenge.ConsumedAt.Valid || time.Now().After(challenge.ExpiresAt) {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "verification_failed", nil)
		return
	}
	if hashEmailVerificationToken(code) != challenge.CodeHash {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "verification_failed", nil)
		return
	}

	var attachCtx passwordAttachChallengeContext
	if err := json.Unmarshal(challenge.RequestContext, &attachCtx); err != nil || attachCtx.PasswordHash == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "verification_failed", nil)
		return
	}

	tx, err := s.repository.BeginTx(r.Context(), nil)
	if err != nil {
		s.log.Errorw("failed to begin password attach transaction", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}
	defer tx.Rollback()

	txq := s.repository.Queries().WithTx(tx)
	identities, err := txq.GetIdentitiesByUserID(r.Context(), challenge.UserID.UUID)
	if err != nil {
		s.log.Errorw("failed to get identities during password attach", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}
	hasPasswordIdentity := false
	for _, identity := range identities {
		if identity.Provider == "password" {
			hasPasswordIdentity = true
			break
		}
	}

	if !hasPasswordIdentity {
		user, userErr := txq.GetUserById(r.Context(), challenge.UserID.UUID)
		if userErr != nil {
			s.log.Errorw("failed to get user during password attach", "error", userErr)
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
			return
		}

		if _, createErr := txq.CreateIdentity(r.Context(), sqlc.CreateIdentityParams{
			UserID:         user.ID,
			Provider:       "password",
			ProviderUserID: email,
			Email:          sql.NullString{String: email, Valid: true},
			EmailVerified:  sql.NullBool{Bool: true, Valid: true},
			Name:           sql.NullString{String: user.UserEmail, Valid: true},
			AvatarUrl:      sql.NullString{Valid: false},
		}); createErr != nil {
			s.log.Errorw("failed to create password identity", "error", createErr)
			if !isUniqueViolation(createErr) {
				pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
				return
			}
		}
	}

	if _, err := txq.UpdatePasswordCredentials(r.Context(), sqlc.UpdatePasswordCredentialsParams{
		UserID:        challenge.UserID.UUID,
		PasswordHash:  attachCtx.PasswordHash,
		PasswordSetBy: "oauth_attach",
	}); err != nil {
		if err == sql.ErrNoRows {
			if _, createErr := txq.CreatePasswordCredentials(r.Context(), sqlc.CreatePasswordCredentialsParams{
				UserID:        challenge.UserID.UUID,
				PasswordHash:  attachCtx.PasswordHash,
				PasswordSetBy: "oauth_attach",
			}); createErr != nil {
				s.log.Errorw("failed to create password credentials during attach", "error", createErr)
				pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
				return
			}
		} else {
			s.log.Errorw("failed to update password credentials during attach", "error", err)
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
			return
		}
	}

	if _, err := txq.ConsumeEmailVerificationChallenge(r.Context(), parsedChallengeID); err != nil {
		s.log.Errorw("failed to consume password attach challenge", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	if err := tx.Commit(); err != nil {
		s.log.Errorw("failed to commit password attach transaction", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "password_attach_success", map[string]any{"attached": true})
}

func (s *APIServer) verifyPasswordResetRequestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.log.Errorw("invalid method for password reset verification", "method", r.Method)
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "method_not_allowed", nil)
		return
	}

	challengeID := chi.URLParam(r, "id")
	if challengeID == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "missing_fields", nil)
		return
	}

	var req struct {
		Email       string `json:"email"`
		Code        string `json:"code"`
		NewPassword string `json:"new_password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.log.Errorw("failed to decode password reset verification request", "error", err)
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_request", nil)
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))
	code := strings.TrimSpace(req.Code)
	newPassword := strings.TrimSpace(req.NewPassword)

	if email == "" || code == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "missing_fields", nil)
		return
	}

	if !emailPattern.MatchString(email) {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_email", nil)
		return
	}

	userIdentity, err := s.passwordAuth.GetUserPasswordIdentity(r.Context(), email)
	if userIdentity == nil || userIdentity.Provider != "password" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "verification_failed", nil)
		return
	}

	parsedChallengeID, err := uuid.Parse(challengeID)
	if err != nil {
		s.log.Errorw("failed to parse password reset challenge id", "error", err)
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_request", nil)
		return
	}

	challenge, err := s.repository.Queries().GetEmailVerificationChallengeById(r.Context(), parsedChallengeID)
	if err != nil {
		s.log.Errorw("failed to load password reset challenge", "error", err)
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "verification_failed", nil)
		return
	}

	if challenge.Purpose != "password_reset" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "verification_failed", nil)
		return
	}

	if strings.TrimSpace(strings.ToLower(challenge.Email)) != email {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "verification_failed", nil)
		return
	}

	if challenge.ConsumedAt.Valid {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "verification_failed", nil)
		return
	}

	if time.Now().After(challenge.ExpiresAt) {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "verification_failed", nil)
		return
	}

	if hashEmailVerificationToken(code) != challenge.CodeHash {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "verification_failed", nil)
		return
	}

	if newPassword == "" {
		pkg.WriteJSONResponse(w, http.StatusOK, "reset_link_verified", map[string]any{"verified": true})
		return
	}

	if len(newPassword) < 8 {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "weak_password", nil)
		return
	}

	user, err := s.repository.Queries().GetUserByEmail(r.Context(), email)
	if err != nil {
		s.log.Errorw("failed to resolve user for password reset", "error", err)
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "verification_failed", nil)
		return
	}

	hashedPassword, err := pkghash.HashPasswordArgon2id(newPassword)
	if err != nil {
		s.log.Errorw("failed to hash reset password", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	tx, err := s.repository.BeginTx(r.Context(), nil)
	if err != nil {
		s.log.Errorw("failed to begin password reset transaction", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}
	defer tx.Rollback()

	txq := s.repository.Queries().WithTx(tx)
	if _, err := txq.UpdatePasswordCredentials(r.Context(), sqlc.UpdatePasswordCredentialsParams{
		UserID:        user.ID,
		PasswordHash:  hashedPassword,
		PasswordSetBy: "password_reset",
	}); err != nil {
		if err == sql.ErrNoRows {
			if _, createErr := txq.CreatePasswordCredentials(r.Context(), sqlc.CreatePasswordCredentialsParams{
				UserID:        user.ID,
				PasswordHash:  hashedPassword,
				PasswordSetBy: "password_reset",
			}); createErr != nil {
				s.log.Errorw("failed to create password credentials during reset", "error", createErr)
				pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
				return
			}
		} else {
			s.log.Errorw("failed to update password credentials during reset", "error", err)
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
			return
		}
	}

	if _, err := txq.ConsumeEmailVerificationChallenge(r.Context(), parsedChallengeID); err != nil {
		s.log.Errorw("failed to consume password reset challenge", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	if err := tx.Commit(); err != nil {
		s.log.Errorw("failed to commit password reset transaction", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "password_reset_success", map[string]any{"password_reset": true})

}

func (s *APIServer) createPasswordResetRequestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.log.Errorw("invalid method for password reset request", "method", r.Method)
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "method_not_allowed", nil)
		return
	}

	var req struct {
		Email string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.log.Errorw("failed to decode password reset request", "error", err)
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_request", nil)
		return
	}

	if req.Email == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "missing_fields", nil)
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if !emailPattern.MatchString(req.Email) {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_email", nil)
		return
	}

	userIdentity, err := s.passwordAuth.GetUserPasswordIdentity(r.Context(), req.Email)
	if userIdentity == nil || userIdentity.Provider != "password" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "verification_failed", nil)
		return
	}

	user, err := s.repository.Queries().GetUserByEmail(r.Context(), req.Email)
	if err == nil {
		token := pkg.GenerateEmailConfirmationCode()
		challenge, createErr := s.repository.Queries().CreateEmailVerificationChallenge(r.Context(), sqlc.CreateEmailVerificationChallengeParams{
			Email:             req.Email,
			UserID:            uuid.NullUUID{UUID: user.ID, Valid: true},
			Purpose:           "password_reset",
			CodeHash:          hashEmailVerificationToken(token),
			ExpiresAt:         time.Now().Add(5 * time.Minute),
			RequestedByIp:     pkg.GetClientIPFromContext(r.Context()),
			RequestContext:    json.RawMessage(`{}`),
			ResendAvailableAt: time.Now().Add(60 * time.Second),
		})
		if createErr != nil {
			s.log.Errorw("failed to create password reset challenge", "error", createErr)
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
			return
		}

		verifyLink := fmt.Sprintf("%s/password/reset/verify/%s?email=%s&code=%s",
			s.backendConfig.FrontendEndpoint,
			url.PathEscape(challenge.ID.String()),
			url.QueryEscape(req.Email),
			url.QueryEscape(token),
		)

		if err := s.passwordAuth.SendPasswordResetEmail(r.Context(), req.Email, verifyLink); err != nil {
			s.log.Errorw("failed to send password reset email", "error", err)
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
			return
		}
	} else if err != sql.ErrNoRows {
		s.log.Errorw("failed to look up user for password reset", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "password_reset_request_success", map[string]any{"sent": true})
}

func (s *APIServer) createRegistrationRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.log.Errorw("invalid method for register", "method", r.Method)
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "method_not_allowed", nil)
		return
	}

	purpose := "register"

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.log.Errorw("failed to decode register request", "error", err)
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_request", nil)
		return
	}

	if req.Email == "" || req.Password == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "missing_fields", nil)
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if !emailPattern.MatchString(req.Email) {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_email", nil)
		return
	}
	if len(req.Password) < 8 {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "weak_password", nil)
		return
	}

	if err := s.passwordAuth.CheckUserExists(r.Context(), req.Email); err != nil {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "forbidden", nil)
		return
	}

	challengeID, token, err := s.passwordAuth.CreatePasswordAuthChallenge(r.Context(), req.Email, purpose, req.Password)
	if err != nil {
		s.log.Errorw("failed to create new user challenge", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	verifyLink := fmt.Sprintf("%s/password/new/verify/%s?email=%s&code=%s",
		s.backendConfig.FrontendEndpoint,
		url.PathEscape(challengeID),
		url.QueryEscape(req.Email),
		url.QueryEscape(token),
	)

	if err := s.passwordAuth.SendConfirmationEmail(r.Context(), req.Email, token, verifyLink); err != nil {
		s.log.Errorw("failed to send confirmation email", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "register_success", map[string]any{"challenge_id": challengeID})

}

func (s *APIServer) verifyRegistrationRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.log.Errorw("invalid method for verify", "method", r.Method)
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "method_not_allowed", nil)
		return
	}

	challengeId := chi.URLParam(r, "id")

	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.log.Errorw("failed to decode verification request", "error", err)
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_request", nil)
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))
	code := strings.TrimSpace(req.Code)

	if email == "" || code == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "missing_fields", nil)
		return
	}

	if !emailPattern.MatchString(email) {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_email", nil)
		return
	}

	if err := s.passwordAuth.ConfirmEmail(r.Context(), email, code, challengeId); err != nil {
		s.log.Errorw("failed to confirm email", "error", err)
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "verification_failed", nil)
		return
	}

	user, err := s.repository.Queries().GetUserByEmail(r.Context(), email)
	if err == nil {
		bucketName := pkg.GetUserBucketName(s.bucketHandler.GetBucketBaseName(), user.ID.String())
		if err := s.repository.Queries().UpdateUserBucketNameById(r.Context(), sqlc.UpdateUserBucketNameByIdParams{
			UserBucket: sql.NullString{String: bucketName, Valid: true},
			ID:         user.ID,
		}); err != nil {
			s.log.Warnw("failed to backfill user bucket name after password verification", "user_id", user.ID, "error", err)
		}
		if err := s.bucketHandler.CreateBucketIfNotExists(r.Context(), user.ID.String()); err != nil {
			s.log.Warnw("failed to create bucket after password verification", "user_id", user.ID, "error", err)
		}
	}
	pkg.WriteJSONResponse(w, http.StatusOK, "", nil)
}

func (s *APIServer) oauthLoginHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	providerName := chi.URLParam(r, "provider")

	provider, ok := s.authProviders[providerName]
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "unknown_provider", nil)
		return
	}

	state, err := generateState()
	if err != nil {
		log.Errorw("failed to generate oauth state", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    state,
		HttpOnly: true,
		Secure:   IsProd,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300, // 5 minutes
	})

	http.Redirect(w, r, provider.GetAuthURL(state), http.StatusTemporaryRedirect)
}

func (s *APIServer) authCallbackHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	ctx := r.Context()

	providerName := chi.URLParam(r, "provider")
	provider, ok := s.authProviders[providerName]
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "unknown_provider", nil)
		return
	}

	// Validate OAuth state to prevent CSRF
	stateCookie, err := r.Cookie(oauthStateCookieName)
	if err != nil || stateCookie.Value == "" {
		http.Redirect(w, r, s.backendConfig.FrontendEndpoint+"?error=invalid_state", http.StatusTemporaryRedirect)
		return
	}
	// Clear the state cookie immediately
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    "",
		HttpOnly: true,
		Secure:   IsProd,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	returnedState := r.URL.Query().Get("state")
	if returnedState == "" || returnedState != stateCookie.Value {
		http.Redirect(w, r, s.backendConfig.FrontendEndpoint+"?error=invalid_state", http.StatusTemporaryRedirect)
		return
	}

	result, err := provider.HandleCallback(ctx, r)
	if err != nil {
		log.Errorw("auth callback error", "provider", providerName, "error", err)
		http.Redirect(w, r, s.backendConfig.FrontendEndpoint+"?error=auth_failed", http.StatusTemporaryRedirect)
		return
	}

	userID, err := s.findOrCreateUserFromResult(ctx, result.Email, result.Provider, result.ProviderUserID, result.EmailVerified, result.Name, result.AvatarURL)
	if err != nil {
		log.Errorw("error finding or creating user", "error", err)
		http.Redirect(w, r, s.backendConfig.FrontendEndpoint+"?error=user_error", http.StatusTemporaryRedirect)
		return
	}

	if err := s.bucketHandler.CreateBucketIfNotExists(ctx, userID.String()); err != nil {
		log.Warnw("failed to create bucket for user", "user", userID, "error", err)
	}

	sessionID := uuid.New()
	encryptedToken := sql.NullString{Valid: false}
	if result.AccessToken != "" {
		enc, err := s.tokenEncryptor.Encrypt(result.AccessToken)
		if err != nil {
			log.Warnw("failed to encrypt provider access token", "error", err)
		} else {
			encryptedToken = sql.NullString{String: enc, Valid: true}
		}
	}
	if _, err := s.repository.Queries().CreateSession(ctx, sqlc.CreateSessionParams{
		ID:                  sessionID,
		UserID:              userID,
		Provider:            providerName,
		ProviderAccessToken: encryptedToken,
		ExpiresAt:           time.Now().Add(sessionDuration),
	}); err != nil {
		log.Errorw("failed to create session", "error", err)
		http.Redirect(w, r, s.backendConfig.FrontendEndpoint+"?error=session_error", http.StatusTemporaryRedirect)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID.String(),
		HttpOnly: true,
		Secure:   IsProd,
		Path:     "/",
		Expires:  time.Now().Add(sessionDuration),
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, s.backendConfig.FrontendEndpoint, http.StatusTemporaryRedirect)
}

// findOrCreateUserFromResult looks up a user by their provider identity, creating one if they don't exist yet.
func (s *APIServer) findOrCreateUserFromResult(ctx context.Context, email, provider, providerUserID string, emailVerified bool, name, avatarURL string) (uuid.UUID, error) {
	// Look up by provider identity first
	identity, err := s.repository.Queries().GetIdentityByProvider(ctx, sqlc.GetIdentityByProviderParams{
		Provider:       provider,
		ProviderUserID: providerUserID,
	})
	if err == nil {
		return identity.UserID, nil
	}
	if err != sql.ErrNoRows {
		return uuid.UUID{}, fmt.Errorf("looking up identity: %w", err)
	}

	// No identity found — check if a user with this email already exists (cross-provider dedup)
	// Only link by email when the provider has verified the address.
	if emailVerified && email != "" {
		existingUser, err := s.repository.Queries().GetUserByEmail(ctx, email)
		if err == nil {
			// User exists under a different provider — attach this identity to them.
			tx, err := s.repository.BeginTx(ctx, nil)
			if err != nil {
				return uuid.UUID{}, fmt.Errorf("beginning transaction: %w", err)
			}
			defer tx.Rollback() //nolint:errcheck

			txq := s.repository.Queries().WithTx(tx)
			if _, err := txq.CreateIdentity(ctx, sqlc.CreateIdentityParams{
				UserID:         existingUser.ID,
				Provider:       provider,
				ProviderUserID: providerUserID,
				Email:          sql.NullString{String: email, Valid: true},
				EmailVerified:  sql.NullBool{Bool: true, Valid: true},
				Name:           sql.NullString{String: name, Valid: name != ""},
				AvatarUrl:      sql.NullString{String: avatarURL, Valid: avatarURL != ""},
			}); err != nil {
				if isUniqueViolation(err) {
					// Concurrent request already linked this identity; re-read to get the right user.
					existing, rerr := s.repository.Queries().GetIdentityByProvider(ctx, sqlc.GetIdentityByProviderParams{
						Provider: provider, ProviderUserID: providerUserID,
					})
					if rerr != nil {
						return uuid.UUID{}, fmt.Errorf("re-reading identity after race: %w", rerr)
					}
					return existing.UserID, nil
				}
				return uuid.UUID{}, fmt.Errorf("linking identity to existing user: %w", err)
			}
			if err := tx.Commit(); err != nil {
				return uuid.UUID{}, fmt.Errorf("committing transaction: %w", err)
			}
			return existingUser.ID, nil
		} else if err != sql.ErrNoRows {
			return uuid.UUID{}, fmt.Errorf("looking up user by email: %w", err)
		}
	}

	// No existing user — create user + identity atomically to avoid orphan rows
	tx, err := s.repository.BeginTx(ctx, nil)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	txq := s.repository.Queries().WithTx(tx)

	user, err := txq.CreateUser(ctx, email)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("creating user: %w", err)
	}

	// Set bucket name using the new UUID
	bucketName := fmt.Sprintf("%s-%s", s.bucketHandler.GetBucketBaseName(), user.ID.String())
	if err := txq.UpdateUserBucketNameById(ctx, sqlc.UpdateUserBucketNameByIdParams{
		UserBucket: sql.NullString{String: bucketName, Valid: true},
		ID:         user.ID,
	}); err != nil {
		logger.FromContext(ctx).Warnw("failed to update bucket name for user", "user", user.ID, "error", err)
	}

	// Create identity
	if _, err := txq.CreateIdentity(ctx, sqlc.CreateIdentityParams{
		UserID:         user.ID,
		Provider:       provider,
		ProviderUserID: providerUserID,
		Email:          sql.NullString{String: email, Valid: email != ""},
		EmailVerified:  sql.NullBool{Bool: emailVerified, Valid: true},
		Name:           sql.NullString{String: name, Valid: name != ""},
		AvatarUrl:      sql.NullString{String: avatarURL, Valid: avatarURL != ""},
	}); err != nil {
		if isUniqueViolation(err) {
			// Concurrent request already created this identity; re-read to converge.
			existing, rerr := s.repository.Queries().GetIdentityByProvider(ctx, sqlc.GetIdentityByProviderParams{
				Provider: provider, ProviderUserID: providerUserID,
			})
			if rerr != nil {
				return uuid.UUID{}, fmt.Errorf("re-reading identity after race: %w", rerr)
			}
			return existing.UserID, nil
		}
		return uuid.UUID{}, fmt.Errorf("creating identity: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return uuid.UUID{}, fmt.Errorf("committing transaction: %w", err)
	}

	return user.ID, nil
}

func (s *APIServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || cookie.Value == "" {
			pkg.WriteJSONResponse(w, http.StatusForbidden, "", "Unauthorized")
			return
		}

		sessionID, err := uuid.Parse(cookie.Value)
		if err != nil {
			pkg.WriteJSONResponse(w, http.StatusForbidden, "", "Invalid session")
			return
		}

		session, err := s.repository.Queries().GetSession(r.Context(), sessionID)
		if err != nil {
			pkg.WriteJSONResponse(w, http.StatusForbidden, "", "Unauthorized (invalid or expired session)")
			return
		}

		user, err := s.repository.Queries().GetUserWithPlan(r.Context(), session.UserID)
		if err != nil {
			log.Errorw("cannot find user", "user_id", session.UserID, "error", err)
			pkg.WriteJSONResponse(w, http.StatusForbidden, "", "User/plan not found")
			return
		}

		identity, err := s.repository.Queries().GetIdentityByUserID(r.Context(), session.UserID)
		if err != nil {
			log.Errorw("cannot find identity for user", "user_id", session.UserID, "error", err)
			pkg.WriteJSONResponse(w, http.StatusForbidden, "", "User identity not found")
			return
		}

		authorizedUser := &userdata.AuthorizedUserInfo{
			UserID:         user.ID.String(),
			ProviderUserID: identity.ProviderUserID,
			Email:          user.UserEmail,
			Name:           identity.Name.String,
			GivenName:      firstWord(identity.Name.String),
			Picture:        identity.AvatarUrl.String,
			Provider:       session.Provider,
		}

		userPlan := &userdata.UserPlan{
			PlanID:                        user.PlanID.String(),
			PlanName:                      user.PlanName,
			MaxFileSizeBytes:              user.MaxFileSizeBytes,
			MaxTotalStorageBytes:          user.MaxTotalStorageBytes,
			MaxFiles:                      user.MaxFiles,
			MaxFilesSentPerDay:            user.MaxFilesSentPerDay,
			MaxSharesPerDay:               user.MaxSharesPerDay,
			MaxUserWorkspaces:             user.MaxUserWorkspaces,
			MaxFilesWorkspace:             user.MaxFilesWorkspace,
			MaxTotalStorageBytesWorkspace: user.MaxTotalStorageBytesWorkspace,
			MaxUsersPerWorkspace:          user.MaxUsersWorkspace,
			MaxWorkspaceFolders:           user.MaxWorkspaceFolders,
			MaxPrivateAPIKeys:             user.MaxPrivateApiKeys,
			MaxWorkspaceAPIKeys:           user.MaxWorkspaceApiKeys,
		}

		ctx := context.WithValue(
			r.Context(),
			userdata.AuthorizedUserWithPlanContextKey,
			&userdata.AuthorizedUserWithPlan{
				AuthorizedUserInfo: *authorizedUser,
				UserPlan:           *userPlan,
			},
		)

		if err := s.bucketHandler.CreateBucketIfNotExists(ctx, user.ID.String()); err != nil {
			log.Warnw("failed to create bucket in middleware", "user", user.ID, "error", err)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *APIServer) logout(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusNotFound, "cookie_not_found", map[string]any{
			"logout_successful": true,
		})
		return
	}

	sessionID, err := uuid.Parse(cookie.Value)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_session", map[string]any{
			"logout_successful": false,
		})
		return
	}

	ctx := r.Context()
	session, err := s.repository.Queries().GetSession(ctx, sessionID)
	if err == nil {
		if provider, exists := s.authProviders[session.Provider]; exists && session.ProviderAccessToken.Valid {
			plainToken, decErr := s.tokenEncryptor.Decrypt(session.ProviderAccessToken.String)
			if decErr != nil {
				log.Warnw("failed to decrypt provider access token", "error", decErr)
			} else if revokeErr := provider.Logout(ctx, plainToken); revokeErr != nil {
				log.Warnw("failed to revoke provider token", "error", revokeErr)
			}
		}
		if deleteErr := s.repository.Queries().DeleteSession(ctx, sessionID); deleteErr != nil {
			log.Warnw("failed to delete session", "error", deleteErr)
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   IsProd,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})

	pkg.WriteJSONResponse(w, http.StatusOK, "session_invalidated", map[string]any{
		"logout_successful": true,
	})
}

func (s *APIServer) isValid(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", map[string]any{
			"authenticated": false,
			"user_info":     nil,
		})
		return
	}

	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "access_denied", map[string]any{
			"authenticated": false,
			"user_info":     nil,
		})
		return
	}

	sessionID, err := uuid.Parse(cookie.Value)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "access_denied", map[string]any{
			"authenticated": false,
			"user_info":     nil,
		})
		return
	}

	session, err := s.repository.Queries().GetSession(r.Context(), sessionID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "access_denied", map[string]any{
			"authenticated": false,
			"user_info":     nil,
		})
		return
	}

	identity, err := s.repository.Queries().GetIdentityByUserID(r.Context(), session.UserID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "access_denied", map[string]any{
			"authenticated": false,
			"user_info":     nil,
		})
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "access_granted", map[string]any{
		"authenticated": true,
		"user_info": map[string]any{
			"sub":            identity.ProviderUserID,
			"email":          identity.Email.String,
			"email_verified": identity.EmailVerified.Valid && identity.EmailVerified.Bool,
			"name":           identity.Name.String,
			"picture":        identity.AvatarUrl.String,
		},
	})
}

func hashEmailVerificationToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// firstWord returns the first space-separated word, or the whole string if no spaces.
func firstWord(s string) string {
	if idx := strings.IndexByte(s, ' '); idx != -1 {
		return s[:idx]
	}
	return s
}

func isUniqueViolation(err error) bool {
	pqErr, ok := err.(*pq.Error)
	return ok && pqErr.Code == "23505"
}
