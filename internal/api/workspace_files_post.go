package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/tscrond/fluxsend-backend/internal/filedata"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

// ── POST /workspaces/{workspace_id}/files/upload  (editor+) ──────────────────

func (s *APIServer) uploadWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
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

	file, header, err := r.FormFile("file")
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "failed_parsing_file")
		return
	}
	defer file.Close()

	folder := wsNormalizePathParam(r.FormValue("folder"))

	if exceedInfo, err := s.validateWorkspaceResourceLimits(r.Context(), workspaceID, header.Size, workspaceQuotaChecks{files: true, storage: true}); err != nil {
		if errors.Is(err, ErrWorkspaceFilesLimitExceeded) || errors.Is(err, ErrWorkspaceStorageLimitExceeded) {
			pkg.WriteJSONResponse(w, http.StatusTooManyRequests, "exceeded_plan_limits", exceedInfo)
		} else {
			log.Printf("[workspace-plan-limit] resource quota check failed for workspace=%s: %v", workspaceID, err)
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		}
		return
	}

	fd := filedata.NewWorkspaceFileData(file, header, header.Filename, folder, workspaceID.String(), callerID.String())
	results, err := s.workspaceFiles.CreateWorkspaceFiles(r.Context(), workspaceID, []filedata.WorkspaceFileData{*fd})
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "upload_failed")
		return
	}
	pkg.WriteJSONResponse(w, http.StatusOK, "uploaded", results)
}

// ── POST /workspaces/{workspace_id}/files/mkdir  (editor+) ───────────────────

func (s *APIServer) mkdirWorkspace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
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
		FolderName string `json:"folder_name"`
		ParentPath string `json:"parent_path"`
	}
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FolderName == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_request")
		return
	}
	// Reject folder names that are not a single clean path segment.
	req.FolderName = strings.TrimSpace(req.FolderName)
	if req.FolderName == "" || req.FolderName == "." || req.FolderName == ".." ||
		strings.ContainsAny(req.FolderName, "/\x00") {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_folder_name")
		return
	}

	if exceedInfo, err := s.validateWorkspaceResourceLimits(r.Context(), workspaceID, 0, workspaceQuotaChecks{folders: true}); err != nil {
		if errors.Is(err, ErrWorkspaceFoldersLimitExceeded) {
			pkg.WriteJSONResponse(w, http.StatusTooManyRequests, "exceeded_plan_limits", exceedInfo)
		} else {
			log.Printf("[workspace-plan-limit] resource quota check failed for workspace=%s: %v", workspaceID, err)
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		}
		return
	}

	result, err := s.workspaceFiles.CreateWorkspaceFolder(r.Context(), workspaceID, callerID, req.FolderName, req.ParentPath)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "mkdir_failed")
		return
	}
	pkg.WriteJSONResponse(w, http.StatusOK, "folder_created", result)
}
