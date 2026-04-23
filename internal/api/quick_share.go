package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/tscrond/fluxsend-backend/pkg"
)

func (s *APIServer) quickShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "bad_request")
		return
	}

	authUser, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
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

	result, err := s.shares.QuickShare(r.Context(), authUser.Email, userUUID, req.Object, req.Duration)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "", "file_not_found")
			return
		}
		log.Println("quickShare error:", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", result)
}

func (s *APIServer) getUnseenReceivedCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "bad_request")
		return
	}

	authUser, _, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	count, err := s.shares.CountUnseen(r.Context(), authUser.Email)
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
	if r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "bad_request")
		return
	}

	authUser, _, ok := parseAuthorizedUserUUID(r)
	if !ok {
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

	if err := s.shares.MarkSeen(r.Context(), authUser.Email, req.SharingToken); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "", "share_not_found")
			return
		}
		log.Println("error marking share seen:", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	log.Printf("share %s marked seen\n", req.SharingToken)
	pkg.WriteJSONResponse(w, http.StatusOK, "", "ok")
}
