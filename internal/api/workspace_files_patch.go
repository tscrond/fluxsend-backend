package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/service"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

// moveWorkspaceFile moves a file within a workspace.
// @Summary Move workspace file
// @Description Moves a file to another path within the same workspace.
// @Tags Workspace Files
// @Accept json
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Param request body object true "Move request"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /api/workspaces/{workspace_id}/files/move [patch]
func (s *CoreHandlers) moveWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
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

	var req struct {
		FileID      string `json:"file_id"`
		Destination string `json:"destination"`
	}
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_request")
		return
	}
	fileID, err := uuid.Parse(req.FileID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_file_id")
		return
	}

	dest := wsNormalizePathParam(req.Destination)
	if err := s.workspaceFiles.MoveWorkspaceFile(r.Context(), workspaceID, fileID, dest, callerID, role); err != nil {
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
	pkg.WriteJSONResponse(w, http.StatusOK, "moved", nil)
}

// moveWorkspaceFolder moves a workspace folder.
// @Summary Move workspace folder
// @Description Moves a folder to a new location inside the workspace.
// @Tags Workspace Files
// @Accept json
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Param request body object true "Move request"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /api/workspaces/{workspace_id}/files/folder/move [patch]
func (s *CoreHandlers) moveWorkspaceFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
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

	var req struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
	}
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Source == "" || req.Destination == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_request")
		return
	}

	src := wsNormalizePathParam(req.Source)
	dst := wsNormalizePathParam(req.Destination)
	if src == "/" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "cannot_move_root")
		return
	}
	if src == dst || dst == src || strings.HasPrefix(dst+"/", src+"/") {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "cannot_move_into_self")
		return
	}

	count, err := s.workspaceFiles.MoveWorkspaceFolder(r.Context(), workspaceID, src, dst, callerID, role)
	if err != nil {
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
	pkg.WriteJSONResponse(w, http.StatusOK, "moved", map[string]int{"updated": count})
}
