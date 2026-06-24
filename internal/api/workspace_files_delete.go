package api

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/service"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

// deleteWorkspaceFile deletes a single file from a workspace.
// @Summary Delete workspace file
// @Description Deletes a workspace file when the caller has write access.
// @Tags Workspace Files
// @Param workspace_id path string true "Workspace ID"
// @Param file_id query string true "File ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /api/workspaces/{workspace_id}/files/delete [delete]
func (s *CoreHandlers) deleteWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
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
	if !wsCanWrite(role) {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}

	fileIDStr := r.URL.Query().Get("file_id")
	fileID, err := uuid.Parse(fileIDStr)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_file_id")
		return
	}

	if err := s.workspaceFiles.RemoveWorkspaceFile(r.Context(), workspaceID, fileID, callerID, role); err != nil {
		if errors.Is(err, service.ErrWsForbidden) {
			pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
			return
		}
		if errors.Is(err, service.ErrWsFileNotFound) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "", "not_found")
			return
		}
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}
	pkg.WriteJSONResponse(w, http.StatusOK, "deleted", nil)
}

// deleteWorkspaceFolder deletes a workspace folder.
// @Summary Delete workspace folder
// @Description Deletes a workspace folder when the caller has write access.
// @Tags Workspace Files
// @Param workspace_id path string true "Workspace ID"
// @Param path query string true "Folder path"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /api/workspaces/{workspace_id}/files/folder/delete [delete]
func (s *CoreHandlers) deleteWorkspaceFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
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
	if !wsCanWrite(role) {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}

	folderPath := wsNormalizePathParam(r.URL.Query().Get("path"))
	if folderPath == "/" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "cannot_delete_root")
		return
	}

	if err := s.workspaceFiles.RemoveWorkspaceFolder(r.Context(), workspaceID, folderPath, callerID, role); err != nil {
		if errors.Is(err, service.ErrWsForbidden) {
			pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
			return
		}
		if errors.Is(err, service.ErrWsFolderNotFound) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "", "not_found")
			return
		}
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}
	pkg.WriteJSONResponse(w, http.StatusOK, "folder_deleted", nil)
}
