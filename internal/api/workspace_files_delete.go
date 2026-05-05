package api

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/service"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

// ── DELETE /workspaces/{workspace_id}/files/delete  (editor own / admin+) ────

func (s *APIServer) deleteWorkspaceFile(w http.ResponseWriter, r *http.Request) {
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

// ── DELETE /workspaces/{workspace_id}/files/folder/delete  (editor own empty / admin+) ──

func (s *APIServer) deleteWorkspaceFolder(w http.ResponseWriter, r *http.Request) {
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
