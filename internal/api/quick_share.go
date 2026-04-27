package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/service"
	"github.com/tscrond/fluxsend-backend/pkg"
)

func (s *APIServer) quickShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "bad_request")
		return
	}

	authUserWithPlan, ok := parseAuthorizedUserWithPlan(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}
	authUser := &authUserWithPlan.AuthorizedUserInfo
	userUUID, err := uuid.Parse(authUser.InternalID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "authorization_failed", "")
		return
	}

	type QuickShareRequest struct {
		Object   string `json:"object"`
		Duration string `json:"duration"`
		Password string `json:"password"`
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

	if exceedInfo, err := s.validateSharePlan(r.Context(), authUser.Email, authUserWithPlan.UserPlan); err != nil {
		if errors.Is(err, ErrDailyShareLimitExceeded) {
			pkg.WriteJSONResponse(w, http.StatusTooManyRequests, "exceeded_plan_limits", exceedInfo)
		} else {
			log.Printf("[plan-limit] share quota check failed for user=%s: %v", authUser.Email, err)
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		}
		return
	}

	result, err := s.shares.QuickShare(r.Context(), authUser.Email, userUUID, req.Object, req.Duration, req.Password)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "", "file_not_found")
			return
		}
		if errors.Is(err, service.ErrPasswordTooLong) {
			pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "password_too_long")
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
