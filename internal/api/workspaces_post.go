package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/pkg"
)

func (s *APIServer) createWorkspace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
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
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "authorization_failed")
		return
	}

	var workspaceResponse struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}

	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&workspaceResponse); err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_json")
		return
	}

	if workspaceResponse.Name == "" || workspaceResponse.Slug == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "name_and_slug_required")
		return
	}

	if err := s.workspaces.CreateWorkspace(
		r.Context(),
		userUUID,
		workspaceResponse.Name,
		workspaceResponse.Slug,
	); err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "workspace_created", map[string]any{
		"workspace": map[string]string{
			"slug": workspaceResponse.Slug,
			"name": workspaceResponse.Name,
		},
	})
}

func (s *APIServer) createWorkspaceInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
		return
	}

	_, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	var req struct {
		WorkspaceID string `json:"workspace_id"`
		Email       string `json:"email"`
		Role        string `json:"role"`
	}

	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_json")
		return
	}

	if err := s.checkIfCanSendInvites(r.Context(), userUUID, uuid.MustParse(req.WorkspaceID)); err != nil {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "unauthorized_to_invite")
		return
	}

	if req.Email == "" || (req.Role != "admin" && req.Role != "editor" && req.Role != "viewer") {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "email_and_valid_role_required")
		return
	}

	workspaceID, err := uuid.Parse(req.WorkspaceID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_workspace_id")
		return
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "token_generation_failed")
		return
	}
	token := hex.EncodeToString(tokenBytes)

	invite, err := s.workspaces.CreateWorkspaceInvite(r.Context(), workspaceID, req.Email, token, req.Role)
	if err != nil {
		if errors.Is(err, errors.New("already_a_member")) || err.Error() == "already_a_member" {
			pkg.WriteJSONResponse(w, http.StatusConflict, "", "already_a_member")
			return
		}
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "invite_created", invite)
}

func (s *APIServer) acceptWorkspaceInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
		return
	}

	authUser, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "token_required")
		return
	}

	if err := s.workspaces.AcceptWorkspaceInvite(r.Context(), req.Token, userUUID, authUser.Email); err != nil {
		switch err.Error() {
		case "invite_not_found":
			pkg.WriteJSONResponse(w, http.StatusNotFound, "", "invite_not_found")
		case "invite_expired":
			pkg.WriteJSONResponse(w, http.StatusGone, "", "invite_expired")
		case "invite_not_for_you":
			pkg.WriteJSONResponse(w, http.StatusForbidden, "", "invite_not_for_you")
		case "already_a_member":
			pkg.WriteJSONResponse(w, http.StatusConflict, "", "already_a_member")
		default:
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		}
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "invite_accepted", nil)
}

func (s *APIServer) checkIfCanSendInvites(ctx context.Context, userUUID uuid.UUID, workspaceID uuid.UUID) error {
	role, err := s.workspaces.GetUserWorkspaceRole(ctx, userUUID, workspaceID)
	if err != nil {
		return err
	}
	if role == "viewer" || role == "editor" {
		log.Println("user is not owner or admin:", role)
		return errors.New("unauthorized_to_invite")
	}

	return nil
}
