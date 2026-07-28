package api

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	storagetypes "github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	"github.com/tscrond/fluxsend-backend/internal/filedata"
	"github.com/tscrond/fluxsend-backend/internal/logger"
	"github.com/tscrond/fluxsend-backend/internal/service"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

func (s *CoreHandlers) initWorkspaceUploadHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", "")
		return
	}

	_, callerID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	workspaceID, role, ok := s.resolveWorkspaceRole(r, callerID)
	if !ok || !wsCanWrite(role) {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}

	var req CreateUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_request", "")
		return
	}
	if req.Size <= 0 {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_file_size", "")
		return
	}

	folder := wsNormalizePathParam(req.Folder)
	if exceedInfo, err := s.validateWorkspaceResourceLimits(r.Context(), workspaceID, req.Size, workspaceQuotaChecks{files: true, storage: true}); err != nil {
		if errors.Is(err, ErrWorkspaceFilesLimitExceeded) || errors.Is(err, ErrWorkspaceStorageLimitExceeded) {
			pkg.WriteJSONResponse(w, http.StatusTooManyRequests, "exceeded_plan_limits", exceedInfo)
		} else {
			log.Errorw("workspace multipart quota check failed", "workspace_id", workspaceID, "error", err)
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		}
		return
	}

	params := &filedata.CreateWorkspaceUploadParams{
		WorkspaceID:    workspaceID,
		UploaderUserID: callerID,
		FileName:       req.Filename,
		Folder:         folder,
		ContentType:    req.ContentType,
		Size:           req.Size,
	}
	uploadResponse, err := s.workspaceFiles.CreateWorkspaceUpload(r.Context(), params)
	if err != nil {
		log.Errorw("failed creating workspace upload id", "workspace_id", workspaceID, "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "failed_creating_upload_id", "")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "created_upload_id", map[string]any{
		"upload_id":  uploadResponse.UploadId,
		"chunk_size": uploadResponse.ChunkSize,
	})
}

func (s *CoreHandlers) uploadWorkspacePartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", "")
		return
	}

	_, callerID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	workspaceID, role, ok := s.resolveWorkspaceRole(r, callerID)
	if !ok || !wsCanWrite(role) {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}

	uploadId := strings.TrimSpace(chi.URLParam(r, "upload_id"))
	if uploadId == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_upload_id", "")
		return
	}
	if r.ContentLength <= 0 {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_part_size", "")
		return
	}

	partIdStr := chi.URLParam(r, "part_id")
	partId, err := strconv.Atoi(partIdStr)
	if err != nil || partId <= 0 || partId > math.MaxInt32 {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_part_number", "")
		return
	}

	result, err := s.workspaceFiles.UploadWorkspacePart(r.Context(), workspaceID, uploadId, int32(partId), r.Body, r.ContentLength)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "error_uploading_part", "")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "uploaded_chunk", result)
}

func (s *CoreHandlers) completeWorkspaceUploadHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", "")
		return
	}

	_, callerID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	workspaceID, role, ok := s.resolveWorkspaceRole(r, callerID)
	if !ok || !wsCanWrite(role) {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}

	uploadId := strings.TrimSpace(chi.URLParam(r, "upload_id"))
	if uploadId == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_upload_id", "")
		return
	}

	result, err := s.workspaceFiles.CompleteWorkspaceUpload(r.Context(), workspaceID, uploadId)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrMultipartUploadIncomplete):
			pkg.WriteJSONResponse(w, http.StatusBadRequest, "multipart_upload_incomplete", "")
		case errors.Is(err, service.ErrMultipartUploadClosed):
			pkg.WriteJSONResponse(w, http.StatusConflict, "multipart_upload_closed", "")
		case errors.Is(err, service.ErrMultipartUploadUnsupported):
			pkg.WriteJSONResponse(w, http.StatusNotImplemented, "multipart_upload_not_supported", "")
		case errors.Is(err, storagetypes.ErrFileAlreadyExists):
			pkg.WriteJSONResponse(w, http.StatusConflict, "file_already_exists", "")
		case strings.Contains(err.Error(), "invalid_upload_id"):
			pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_upload_id", "")
		default:
			log.Errorw("error completing workspace multipart upload", "workspace_id", workspaceID, "upload_id", uploadId, "error", err)
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "error_completing_upload", "")
		}
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "upload_completed", result)
}

func (s *CoreHandlers) abortWorkspaceUploadHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodDelete {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", "")
		return
	}

	_, callerID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	workspaceID, role, ok := s.resolveWorkspaceRole(r, callerID)
	if !ok || !wsCanWrite(role) {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}

	uploadId := strings.TrimSpace(chi.URLParam(r, "upload_id"))
	if uploadId == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_upload_id", "")
		return
	}

	result, err := s.workspaceFiles.AbortWorkspaceUpload(r.Context(), workspaceID, uploadId)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrMultipartUploadUnsupported):
			pkg.WriteJSONResponse(w, http.StatusNotImplemented, "multipart_upload_not_supported", "")
		case strings.Contains(err.Error(), "invalid_upload_id"):
			pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_upload_id", "")
		default:
			log.Errorw("error aborting workspace multipart upload", "workspace_id", workspaceID, "upload_id", uploadId, "error", err)
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "error_aborting_upload", "")
		}
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "upload_aborted", result)
}

// uploadWorkspaceFile uploads a file into a workspace.
// @Summary Upload workspace file
// @Description Uploads a file into a workspace folder for users with write access.
// @Tags Workspace Files
// @Accept multipart/form-data
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Param file formData file true "File to upload"
// @Param folder formData string false "Destination folder"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Failure 429 {object} map[string]any
// @Router /api/workspaces/{workspace_id}/files/upload [post]
func (s *CoreHandlers) uploadWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
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
			log.Errorw("workspace resource quota check failed", "workspace_id", workspaceID, "error", err)
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

// mkdirWorkspace creates a new folder in a workspace.
// @Summary Create workspace folder
// @Description Creates a new folder inside the selected workspace.
// @Tags Workspace Files
// @Accept json
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Param request body object true "Folder creation request"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Failure 429 {object} map[string]any
// @Router /api/workspaces/{workspace_id}/files/mkdir [post]
func (s *CoreHandlers) mkdirWorkspace(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
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
			log.Errorw("workspace folder quota check failed", "workspace_id", workspaceID, "error", err)
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
