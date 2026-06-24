package api

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/logger"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

// listWorkspaces returns all workspaces visible to the current user.
// @Summary List workspaces
// @Description Returns all workspaces visible to the authenticated user.
// @Tags Workspaces
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Router /api/workspaces/list [get]
func (s *CoreHandlers) listWorkspaces(w http.ResponseWriter, r *http.Request) {
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

// getWorkspaceMembers lists members in a workspace.
// @Summary List workspace members
// @Description Returns the members of a workspace when the caller has access to it.
// @Tags Workspaces
// @Param workspace_id query string true "Workspace ID"
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Router /api/workspaces/members [get]
func (s *CoreHandlers) getWorkspaceMembers(w http.ResponseWriter, r *http.Request) {
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

// getWorkspaceInvites lists pending invites for a workspace.
// @Summary List workspace invites
// @Description Returns pending invites for a workspace when the caller is allowed to view them.
// @Tags Workspaces
// @Param workspace_id query string true "Workspace ID"
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Router /api/workspaces/invites [get]
func (s *CoreHandlers) getWorkspaceInvites(w http.ResponseWriter, r *http.Request) {
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

// getMyWorkspaceInvites lists workspace invites for the current user.
// @Summary List my workspace invites
// @Description Returns all workspace invites associated with the authenticated user.
// @Tags Workspaces
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Router /api/workspaces/invites/mine [get]
func (s *CoreHandlers) getMyWorkspaceInvites(w http.ResponseWriter, r *http.Request) {
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
// getWorkspaceQuota returns workspace usage and quota details.
// @Summary Get workspace quota
// @Description Returns storage, file, folder, member, and API key quota information for a workspace.
// @Tags Workspaces
// @Param workspace_id path string true "Workspace ID"
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Router /api/workspaces/{workspace_id}/quota [get]
func (s *CoreHandlers) getWorkspaceQuota(w http.ResponseWriter, r *http.Request) {
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
		"api_key_count":                     row.ApiKeyCount,
		"max_files_workspace":               row.MaxFilesWorkspace,
		"max_total_storage_bytes_workspace": row.MaxTotalStorageBytesWorkspace,
		"max_users_workspace":               row.MaxUsersWorkspace,
		"max_workspace_folders":             row.MaxWorkspaceFolders,
		"max_workspace_api_keys":            row.MaxWorkspaceApiKeys,
	})
}
