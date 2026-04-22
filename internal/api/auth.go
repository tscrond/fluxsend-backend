package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"github.com/tscrond/fluxsend-backend/internal/userdata"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

const (
	IsProd               = true
	sessionCookieName    = "session_id"
	oauthStateCookieName = "oauth_state"
	sessionDuration      = 24 * time.Hour
)

func (s *APIServer) oauthLoginHandler(w http.ResponseWriter, r *http.Request) {
	providerName := chi.URLParam(r, "provider")
	provider, ok := s.authProviders[providerName]
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "unknown_provider", nil)
		return
	}

	state, err := generateState()
	if err != nil {
		log.Printf("failed to generate oauth state: %v", err)
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
		log.Printf("auth callback error [%s]: %v", providerName, err)
		http.Redirect(w, r, s.backendConfig.FrontendEndpoint+"?error=auth_failed", http.StatusTemporaryRedirect)
		return
	}

	userID, err := s.findOrCreateUserFromResult(ctx, result.Email, result.Provider, result.ProviderUserID, result.EmailVerified, result.Name, result.AvatarURL)
	if err != nil {
		log.Printf("error finding or creating user: %v", err)
		http.Redirect(w, r, s.backendConfig.FrontendEndpoint+"?error=user_error", http.StatusTemporaryRedirect)
		return
	}

	if err := s.bucketHandler.CreateBucketIfNotExists(ctx, userID.String()); err != nil {
		log.Printf("warning: failed to create bucket for user %s: %v", userID, err)
	}

	sessionID := uuid.New()
	if _, err := s.repository.Queries.CreateSession(ctx, sqlc.CreateSessionParams{
		ID:                  sessionID,
		UserID:              userID,
		Provider:            providerName,
		ProviderAccessToken: sql.NullString{String: result.AccessToken, Valid: result.AccessToken != ""},
		ExpiresAt:           time.Now().Add(sessionDuration),
	}); err != nil {
		log.Printf("failed to create session: %v", err)
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
	identity, err := s.repository.Queries.GetIdentityByProvider(ctx, sqlc.GetIdentityByProviderParams{
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
		existingUser, err := s.repository.Queries.GetUserByEmail(ctx, email)
		if err == nil {
			// User exists under a different provider — attach this identity to them.
			tx, err := s.repository.BeginTx(ctx)
			if err != nil {
				return uuid.UUID{}, fmt.Errorf("beginning transaction: %w", err)
			}
			defer tx.Rollback() //nolint:errcheck

			txq := s.repository.Queries.WithTx(tx)
			if _, err := txq.CreateIdentity(ctx, sqlc.CreateIdentityParams{
				UserID:         existingUser.ID,
				Provider:       provider,
				ProviderUserID: providerUserID,
				Email:          sql.NullString{String: email, Valid: true},
				EmailVerified:  sql.NullBool{Bool: true, Valid: true},
				Name:           sql.NullString{String: name, Valid: name != ""},
				AvatarUrl:      sql.NullString{String: avatarURL, Valid: avatarURL != ""},
			}); err != nil {
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
	tx, err := s.repository.BeginTx(ctx)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	txq := s.repository.Queries.WithTx(tx)

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
		log.Printf("warning: failed to update bucket name for user %s: %v", user.ID, err)
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
		return uuid.UUID{}, fmt.Errorf("creating identity: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return uuid.UUID{}, fmt.Errorf("committing transaction: %w", err)
	}

	return user.ID, nil
}

func (s *APIServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		session, err := s.repository.Queries.GetSession(r.Context(), sessionID)
		if err != nil {
			pkg.WriteJSONResponse(w, http.StatusForbidden, "", "Unauthorized (invalid or expired session)")
			return
		}

		user, err := s.repository.Queries.GetUserById(r.Context(), session.UserID)
		if err != nil {
			log.Printf("cannot find user %s: %v", session.UserID, err)
			pkg.WriteJSONResponse(w, http.StatusForbidden, "", "User not found")
			return
		}

		identity, err := s.repository.Queries.GetIdentityByUserID(r.Context(), session.UserID)
		if err != nil {
			log.Printf("cannot find identity for user %s: %v", session.UserID, err)
			pkg.WriteJSONResponse(w, http.StatusForbidden, "", "User identity not found")
			return
		}

		authorizedUser := &userdata.AuthorizedUserInfo{
			InternalID: user.ID.String(),
			Id:         identity.ProviderUserID,
			Email:      user.UserEmail,
			Name:       identity.Name.String,
			GivenName:  firstWord(identity.Name.String),
			Picture:    identity.AvatarUrl.String,
			Provider:   session.Provider,
		}

		ctx := context.WithValue(r.Context(), userdata.AuthorizedUserContextKey, authorizedUser)

		if err := s.bucketHandler.CreateBucketIfNotExists(ctx, user.ID.String()); err != nil {
			log.Printf("warning: failed to create bucket in middleware: %v", err)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *APIServer) logout(w http.ResponseWriter, r *http.Request) {
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
	session, err := s.repository.Queries.GetSession(ctx, sessionID)
	if err == nil {
		if provider, exists := s.authProviders[session.Provider]; exists && session.ProviderAccessToken.Valid {
			if revokeErr := provider.Logout(ctx, session.ProviderAccessToken.String); revokeErr != nil {
				log.Printf("warning: failed to revoke provider token: %v", revokeErr)
			}
		}
		if deleteErr := s.repository.Queries.DeleteSession(ctx, sessionID); deleteErr != nil {
			log.Printf("warning: failed to delete session: %v", deleteErr)
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

	session, err := s.repository.Queries.GetSession(r.Context(), sessionID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "access_denied", map[string]any{
			"authenticated": false,
			"user_info":     nil,
		})
		return
	}

	identity, err := s.repository.Queries.GetIdentityByUserID(r.Context(), session.UserID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "access_denied", map[string]any{
			"authenticated": false,
			"user_info":     nil,
		})
		return
	}

	emailVerified := "false"
	if identity.EmailVerified.Valid && identity.EmailVerified.Bool {
		emailVerified = "true"
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "access_granted", map[string]any{
		"authenticated": true,
		"user_info": map[string]any{
			"sub":            identity.ProviderUserID,
			"email":          identity.Email.String,
			"email_verified": emailVerified,
			"name":           identity.Name.String,
			"picture":        identity.AvatarUrl.String,
		},
	})
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
