package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/tscrond/fluxsend-backend/pkg"
)

func (s *APIServer) shareWith(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "bad_request")
		return
	}

	authUser, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
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
