package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"github.com/tscrond/fluxsend-backend/internal/userdata"
	"github.com/tscrond/fluxsend-backend/pkg"
)

func (s *APIServer) quickShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "bad_request")
		return
	}

	authorizedUserData := ctx.Value(userdata.AuthorizedUserContextKey)
	authUserData, ok := authorizedUserData.(*userdata.AuthorizedUserInfo)
	if !ok {
		log.Println("cannot read authorized user data")
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	type QuickShareRequest struct {
		Object   string `json:"object"`
		Duration string `json:"duration"`
	}

	var req QuickShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_json")
		return
	}

	if req.Object == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "object is required")
		return
	}

	if req.Duration == "" {
		req.Duration = "24h"
	}

	expiryDuration, err := pkg.CustomParseDuration(req.Duration)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid duration parameter")
		return
	}

	// Look up the file owned by this user
	fileData, err := s.repository.Queries.GetFileByOwnerAndName(ctx, sqlc.GetFileByOwnerAndNameParams{
		OwnerGoogleID: sql.NullString{Valid: true, String: authUserData.Id},
		FileName:      req.Object,
	})
	if err != nil {
		log.Println("error getting object data for quick share:", err)
		pkg.WriteJSONResponse(w, http.StatusNotFound, "", "file_not_found")
		return
	}

	// Check for an existing unexpired public link
	existingShare, err := s.repository.Queries.GetExistingPublicShare(ctx, sqlc.GetExistingPublicShareParams{
		SharedBy: sql.NullString{Valid: true, String: authUserData.Email},
		FileID:   sql.NullInt32{Valid: true, Int32: fileData.ID},
	})
	if err == nil {
		// Reuse existing public link
		pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
			"sharing_token": existingShare.SharingToken,
			"expires_at":    existingShare.ExpiresAt,
			"sharing_link":  fmt.Sprintf("%s/d/%s", s.backendConfig.BackendEndpoint, existingShare.SharingToken),
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		log.Println("error checking existing public share:", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	// Create a new public share
	generatedToken, _ := pkg.RandToken(32)
	expiresAt := time.Now().Add(expiryDuration)

	share, err := s.repository.Queries.InsertPublicShare(ctx, sqlc.InsertPublicShareParams{
		SharedBy:     sql.NullString{Valid: true, String: authUserData.Email},
		FileID:       sql.NullInt32{Valid: true, Int32: fileData.ID},
		ExpiresAt:    expiresAt,
		SharingToken: generatedToken,
	})
	if err != nil {
		log.Println("error inserting public share:", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "sharing_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"sharing_token": share.SharingToken,
		"expires_at":    share.ExpiresAt,
		"sharing_link":  fmt.Sprintf("%s/d/%s", s.backendConfig.BackendEndpoint, share.SharingToken),
	})
}

func (s *APIServer) getUnseenReceivedCount(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "bad_request")
		return
	}

	authorizedUserData := ctx.Value(userdata.AuthorizedUserContextKey)
	authUserData, ok := authorizedUserData.(*userdata.AuthorizedUserInfo)
	if !ok {
		log.Println("cannot read authorized user data")
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	count, err := s.repository.Queries.CountUnseenShares(ctx, sql.NullString{Valid: true, String: authUserData.Email})
	if err != nil {
		log.Println("error counting unseen shares:", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"count": count,
	})
}

func (s *APIServer) markReceivedSeen(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "bad_request")
		return
	}

	authorizedUserData := ctx.Value(userdata.AuthorizedUserContextKey)
	authUserInfo, ok := authorizedUserData.(*userdata.AuthorizedUserInfo)
	if !ok {
		log.Println("cannot read authorized user data")
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	type MarkSeenRequest struct {
		SharingToken string `json:"sharing_token"`
	}

	var req MarkSeenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_json")
		return
	}

	if req.SharingToken == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "sharing_token is required")
		return
	}

	if err := s.repository.Queries.MarkShareSeen(ctx, sqlc.MarkShareSeenParams{
		SharingToken: req.SharingToken,
		SharedFor: sql.NullString{
			String: authUserInfo.Email,
			Valid:  true,
		},
	}); err != nil {
		log.Println("error marking share seen:", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", "ok")
}
