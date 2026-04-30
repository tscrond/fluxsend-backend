package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/service"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

// ── PATCH /workspaces/{workspace_id}/files/move  (editor own / admin+) ───────

func (s *APIServer) moveWorkspaceFile(w http.ResponseWriter, r *http.Request) {
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

// ── PATCH /workspaces/{workspace_id}/files/folder/move  (editor own / admin+) ──

func (s *APIServer) moveWorkspaceFolder(w http.ResponseWriter, r *http.Request) {
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
