package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

func (s *CoreHandlers) renameWorkspace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
		return
	}

	_, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	var body struct {
		WorkspaceID string `json:"workspace_id"`
		Name        string `json:"name"`
		Slug        string `json:"slug"`
	}

	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_json")
		return
	}

	if body.Name == "" || body.WorkspaceID == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "workspace_id_and_name_required")
		return
	}

	if len([]rune(body.Name)) > 64 {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "name_too_long")
		return
	}

	if len([]rune(body.Slug)) > 48 {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "slug_too_long")
		return
	}

	workspaceID, err := uuid.Parse(body.WorkspaceID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_workspace_id")
		return
	}

	role, err := s.workspaces.GetUserWorkspaceRole(r.Context(), userUUID, workspaceID)
	if err != nil || (role != "owner" && role != "admin") {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}

	updated, err := s.workspaces.RenameWorkspace(r.Context(), workspaceID, body.Name, body.Slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "", "workspace_not_found")
			return
		}
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "workspace_renamed", updated)
}

func (s *CoreHandlers) changeMemberRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
		return
	}

	_, callerUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	var body struct {
		WorkspaceID string `json:"workspace_id"`
		UserID      string `json:"user_id"`
		Role        string `json:"role"`
	}

	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_json")
		return
	}

	if body.WorkspaceID == "" || body.UserID == "" || body.Role == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "workspace_id_user_id_and_role_required")
		return
	}

	validRoles := map[string]bool{"owner": true, "admin": true, "editor": true, "viewer": true}
	if !validRoles[body.Role] {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_role")
		return
	}

	workspaceID, err := uuid.Parse(body.WorkspaceID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_workspace_id")
		return
	}

	targetUserID, err := uuid.Parse(body.UserID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_user_id")
		return
	}

	callerRole, err := s.workspaces.GetUserWorkspaceRole(r.Context(), callerUUID, workspaceID)
	if err != nil || (callerRole != "owner" && callerRole != "admin") {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}

	// Only owners can assign the owner role
	if body.Role == "owner" && callerRole != "owner" {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "only_owner_can_assign_owner_role")
		return
	}

	// Admins cannot change the role of an owner
	targetRole, err := s.workspaces.GetUserWorkspaceRole(r.Context(), targetUserID, workspaceID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusNotFound, "", "target_member_not_found")
		return
	}
	if targetRole == "owner" && callerRole != "owner" {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "cannot_change_owner_role")
		return
	}

	if err := s.workspaces.UpdateMemberRole(r.Context(), workspaceID, targetUserID, body.Role); err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "member_role_updated", map[string]string{
		"user_id": body.UserID,
		"role":    body.Role,
	})
}
