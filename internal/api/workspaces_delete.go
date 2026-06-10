package api

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/google/uuid"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

func (s *CoreHandlers) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
		return
	}

	_, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	workspaceIDStr := r.URL.Query().Get("workspace_id")
	workspaceID, err := uuid.Parse(workspaceIDStr)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_workspace_id")
		return
	}

	role, err := s.workspaces.GetUserWorkspaceRole(r.Context(), userUUID, workspaceID)
	if err != nil || role != "owner" {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}

	if err := s.workspaces.DeleteWorkspace(r.Context(), workspaceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "", "workspace_not_found")
			return
		}
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "workspace_deleted", map[string]string{
		"workspace_id": workspaceID.String(),
	})
}

func (s *CoreHandlers) deleteWorkspaceInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
		return
	}

	_, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	inviteIDStr := r.URL.Query().Get("invite_id")
	inviteID, err := uuid.Parse(inviteIDStr)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_invite_id")
		return
	}

	workspaceIDStr := r.URL.Query().Get("workspace_id")
	workspaceID, err := uuid.Parse(workspaceIDStr)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_workspace_id")
		return
	}

	role, err := s.workspaces.GetUserWorkspaceRole(r.Context(), userUUID, workspaceID)
	if err != nil || (role != "owner" && role != "admin") {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}

	if err := s.workspaces.DeleteWorkspaceInvite(r.Context(), inviteID); err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "invite_deleted", map[string]string{
		"invite_id": inviteID.String(),
	})
}

func (s *CoreHandlers) rejectWorkspaceInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
		return
	}

	authUser, _, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "token_required")
		return
	}

	if err := s.workspaces.RejectWorkspaceInvite(r.Context(), token, authUser.Email); err != nil {
		switch err.Error() {
		case "invite_not_found":
			pkg.WriteJSONResponse(w, http.StatusNotFound, "", "invite_not_found")
		case "invite_not_for_you":
			pkg.WriteJSONResponse(w, http.StatusForbidden, "", "invite_not_for_you")
		default:
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		}
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "invite_rejected", nil)
}

func (s *CoreHandlers) removeWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
		return
	}
	_, requesterUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	workspaceIDStr := r.URL.Query().Get("workspace_id")
	workspaceID, err := uuid.Parse(workspaceIDStr)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_workspace_id")
		return
	}

	userIDStr := r.URL.Query().Get("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_user_id")
		return
	}

	// Only owner/admin may remove members
	requesterRole, err := s.workspaces.GetUserWorkspaceRole(r.Context(), requesterUUID, workspaceID)
	if err != nil || (requesterRole != "owner" && requesterRole != "admin") {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}
	// Owners cannot be removed
	targetRole, err := s.workspaces.GetUserWorkspaceRole(r.Context(), userID, workspaceID)
	if err == nil && targetRole == "owner" {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "cannot_remove_owner")
		return
	}

	if err := s.workspaces.RemoveWorkspaceMember(r.Context(), workspaceID, userID); err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "member_removed", nil)
}
