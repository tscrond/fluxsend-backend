package api

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/logger"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

func (s *APIServer) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
		return
	}

	_, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	workspaces, err := s.workspaces.GetUserWorkspaces(r.Context(), userUUID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "ok", workspaces)
}

func (s *APIServer) getWorkspaceMembers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
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

	if _, err := s.workspaces.GetUserWorkspaceRole(r.Context(), userUUID, workspaceID); err != nil {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}

	members, err := s.workspaces.GetWorkspaceMembers(r.Context(), workspaceID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "ok", members)
}

func (s *APIServer) getWorkspaceInvites(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
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
	if err != nil || (role != "owner" && role != "admin") {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}

	invites, err := s.workspaces.GetWorkspaceInvites(r.Context(), workspaceID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "ok", invites)
}

func (s *APIServer) getMyWorkspaceInvites(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
		return
	}

	authUser, _, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	invites, err := s.workspaces.GetUserInvites(r.Context(), authUser.Email)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "ok", invites)
}

// GET /workspaces/{workspace_id}/quota  (admin/owner only)
func (s *APIServer) getWorkspaceQuota(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
		return
	}

	_, callerID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	workspaceID, role, ok := s.resolveWorkspaceRole(r, callerID)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}
	if role != "owner" && role != "admin" {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}

	row, err := s.repository.Queries().GetWorkspaceQuotaDetails(r.Context(), workspaceID)
	if err != nil {
		log.Errorw("DB error fetching workspace quota", "workspace_id", workspaceID, "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "ok", map[string]any{
		"file_count":                        row.FileCount,
		"total_bytes":                       row.TotalBytes,
		"folder_count":                      row.FolderCount,
		"member_count":                      row.MemberCount,
		"max_files_workspace":               row.MaxFilesWorkspace,
		"max_total_storage_bytes_workspace": row.MaxTotalStorageBytesWorkspace,
		"max_users_workspace":               row.MaxUsersWorkspace,
		"max_workspace_folders":             row.MaxWorkspaceFolders,
	})
}
