package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/pkg"
)

func (s *APIServer) shareWith(w http.ResponseWriter, r *http.Request) {
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

	type ShareRequest struct {
		ForUser   string   `json:"email"`
		Objects   []string `json:"objects"`
		Duration  string   `json:"duration"`
		SendEmail bool     `json:"send_email"`
	}
	var req ShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_json")
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

	shares, notificationStatus, err := s.shares.ShareWith(r.Context(), authUser.Email, userUUID, req.ForUser, req.Objects, req.Duration, req.SendEmail)
	if err != nil {
		log.Println("shareWith error:", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "sharing_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"sharing_info":        shares,
		"notification_status": notificationStatus,
	})
}
