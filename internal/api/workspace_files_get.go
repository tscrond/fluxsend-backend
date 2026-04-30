package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/service"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

// ── GET /workspaces/{workspace_id}/files/tree  (viewer+) ─────────────────────

func (s *APIServer) getWorkspaceFilesTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
		return
	}

	_, callerID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	workspaceID, _, ok := s.resolveWorkspaceRole(r, callerID)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}

	path := wsNormalizePathParam(r.URL.Query().Get("path"))
	tree, err := s.workspaceFiles.GetWorkspaceFilesTree(r.Context(), workspaceID, path)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}
	pkg.WriteJSONResponse(w, http.StatusOK, "", tree)
}

// ── GET /workspaces/{workspace_id}/files/download  (viewer+) ─────────────────

func (s *APIServer) downloadWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
		return
	}

	_, callerID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	workspaceID, _, ok := s.resolveWorkspaceRole(r, callerID)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}

	fileIDStr := r.URL.Query().Get("file_id")
	fileID, err := uuid.Parse(fileIDStr)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_file_id")
		return
	}

	mode := r.URL.Query().Get("mode")
	if mode != "inline" && mode != "download" && mode != "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_mode")
		return
	}
	if mode == "" {
		mode = "download"
	}

	info, err := s.workspaceFiles.GetWorkspaceFileDownloadInfo(r.Context(), workspaceID, fileID)
	if err != nil {
		if errors.Is(err, service.ErrWsFileNotFound) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "", "not_found")
			return
		}
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	expiresAt := time.Now().Add(time.Minute)
	var contentDisposition string
	if mode == "download" {
		contentDisposition = "attachment; filename=" + info.FileName
	}

	signedURL, err := s.bucketHandler.GenerateSignedURL(r.Context(), info.Bucket, info.ObjectKey, expiresAt, contentDisposition)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	s.handleDownloadResponse(w, r, signedURL, info.FileName, mode)
}
