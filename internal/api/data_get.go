package api

import (
	"net/http"
	"time"

	"github.com/tscrond/fluxsend-backend/internal/logger"
	"github.com/tscrond/fluxsend-backend/pkg"
)

// getUserData returns the authenticated user's account and plan information.
// @Summary Get current user data
// @Description Returns the authenticated user's profile and plan details.
// @Tags User
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Router /api/user/data [get]
func (s *CoreHandlers) getUserData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "bad_request")
		return
	}

	uwp, ok := parseAuthorizedUserWithPlan(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "Access Denied", map[string]any{
			"user_data": nil,
		})
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"user_data": uwp,
	})
}

// getUserBucketData returns bucket-level metadata for the authenticated user.
// @Summary Get user bucket data
// @Description Returns the current user's bucket metadata and usage details.
// @Tags User
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Router /api/user/bucket [get]
func (s *CoreHandlers) getUserBucketData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "bad_request")
		return
	}

	_, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "access_denied", map[string]any{
			"bucket_data": nil,
		})
		return
	}

	bucketData, err := s.users.GetBucketData(r.Context(), userUUID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", map[string]any{
			"bucket_data": nil,
		})
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "bucket_data_retrieved", map[string]any{
		"bucket_data": bucketData,
	})
}

// getUserIdentities returns every linked identity for the authenticated user.
// @Summary Get linked identities
// @Description Returns all identities attached to the current account.
// @Tags User
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Router /api/user/identities [get]
func (s *CoreHandlers) getUserIdentities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "method_not_allowed", nil)
		return
	}

	_, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "access_denied", map[string]any{"identities": nil})
		return
	}

	identities, err := s.repository.Queries().GetIdentitiesByUserID(r.Context(), userUUID)
	if err != nil {
		logger.FromContext(r.Context()).Errorw("failed to load user identities", "user_id", userUUID, "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", map[string]any{"identities": nil})
		return
	}

	type identityResponse struct {
		ID             string    `json:"id"`
		UserID         string    `json:"user_id"`
		Provider       string    `json:"provider"`
		ProviderUserID string    `json:"provider_user_id"`
		Email          string    `json:"email"`
		EmailVerified  bool      `json:"email_verified"`
		Name           string    `json:"name"`
		AvatarURL      string    `json:"avatar_url"`
		CreatedAt      time.Time `json:"created_at"`
	}

	response := make([]identityResponse, 0, len(identities))
	hasPasswordIdentity := false
	for _, identity := range identities {
		createdAt := time.Time{}
		if identity.CreatedAt.Valid {
			createdAt = identity.CreatedAt.Time
		}
		if identity.Provider == "password" {
			hasPasswordIdentity = true
		}
		response = append(response, identityResponse{
			ID:             identity.ID.String(),
			UserID:         identity.UserID.String(),
			Provider:       identity.Provider,
			ProviderUserID: identity.ProviderUserID,
			Email:          identity.Email.String,
			EmailVerified:  identity.EmailVerified.Valid && identity.EmailVerified.Bool,
			Name:           identity.Name.String,
			AvatarURL:      identity.AvatarUrl.String,
			CreatedAt:      createdAt,
		})
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "identities_retrieved", map[string]any{
		"identities":            response,
		"has_password_identity": hasPasswordIdentity,
	})
}

// getUserPrivateFileByName generates a personal download token for a private file.
// @Summary Create private download token
// @Description Generates a download token for a private file name.
// @Tags User
// @Param file query string true "File name"
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Router /api/user/private/download_token [post]
func (s *CoreHandlers) getUserPrivateFileByName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", "")
		return
	}

	_, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "access_denied", "")
		return
	}

	fileName := r.URL.Query().Get("file")

	token, err := s.users.GetPrivateDownloadToken(r.Context(), userUUID, fileName)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"private_download_token": token,
	})
}

// GET /user/stats — personal usage analytics
// getUserStats returns aggregate usage statistics for the authenticated user.
// @Summary Get user statistics
// @Description Returns aggregate file, share, and workspace statistics for the current user.
// @Tags User
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Router /api/user/stats [get]
func (s *CoreHandlers) getUserStats(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
		return
	}

	_, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "access_denied")
		return
	}

	stats, err := s.repository.Queries().GetUserStats(r.Context(), userUUID)
	if err != nil {
		log.Errorw("error fetching user stats", "user", userUUID, "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	dailyUploads, err := s.repository.Queries().GetUserDailyUploads(r.Context(), userUUID)
	if err != nil {
		log.Errorw("error fetching daily uploads", "user", userUUID, "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	dailyShares, err := s.repository.Queries().GetUserDailyShares(r.Context(), userUUID)
	if err != nil {
		log.Errorw("error fetching daily shares", "user", userUUID, "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "ok", map[string]any{
		"total_files":        stats.TotalFiles,
		"total_bytes":        stats.TotalBytes,
		"files_today":        stats.FilesToday,
		"files_last_7d":      stats.FilesLast7d,
		"files_last_30d":     stats.FilesLast30d,
		"total_shares_sent":  stats.TotalSharesSent,
		"shares_today":       stats.SharesToday,
		"targeted_shares":    stats.TargetedShares,
		"public_shares":      stats.PublicShares,
		"active_shares":      stats.ActiveShares,
		"total_received":     stats.TotalReceived,
		"owned_workspaces":   stats.OwnedWorkspaces,
		"daily_uploads":      dailyUploads,
		"daily_shares":       dailyShares,
		"workspace_api_keys": stats.WorkspaceApiKeysUsed,
	})
}
