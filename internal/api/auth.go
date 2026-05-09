package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/tscrond/fluxsend-backend/internal/logger"
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
			InternalID: user.ID.String(),
			Id:         identity.ProviderUserID,
			Email:      user.UserEmail,
			Name:       identity.Name.String,
			GivenName:  firstWord(identity.Name.String),
			Picture:    identity.AvatarUrl.String,
			Provider:   session.Provider,
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
