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

// getWorkspaceFilesTree returns the tree for a workspace path.
// @Summary Get workspace file tree
// @Description Returns the file tree for a workspace path visible to the caller.
// @Tags Workspace Files
// @Param workspace_id path string true "Workspace ID"
// @Param path query string false "Path prefix"
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Router /api/workspaces/{workspace_id}/files/tree [get]
func (s *CoreHandlers) getWorkspaceFilesTree(w http.ResponseWriter, r *http.Request) {
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

// downloadWorkspaceFile creates a signed download URL for a workspace file.
// @Summary Download workspace file
// @Description Creates a signed download URL for a workspace file and redirects or proxies based on mode.
// @Tags Workspace Files
// @Param workspace_id path string true "Workspace ID"
// @Param file_id query string true "File ID"
// @Param mode query string false "inline or download"
// @Success 302 "Redirect to the signed URL"
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /api/workspaces/{workspace_id}/files/download [get]
func (s *CoreHandlers) downloadWorkspaceFile(w http.ResponseWriter, r *http.Request) {
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
		contentDisposition = buildAttachmentContentDisposition(info.FileName)
	}

	signedURL, err := s.bucketHandler.GenerateSignedURL(r.Context(), info.Bucket, info.ObjectKey, expiresAt, contentDisposition)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	s.handleDownloadResponse(w, r, signedURL, info.FileName, mode)
}
