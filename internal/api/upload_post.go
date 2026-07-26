package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	storagetypes "github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	"github.com/tscrond/fluxsend-backend/internal/filedata"
	"github.com/tscrond/fluxsend-backend/internal/logger"
	"github.com/tscrond/fluxsend-backend/internal/service"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

type CreateUploadRequest struct {
	Filename    string `json:"filename"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	Folder      string `json:"folder"`
}

func (s *CoreHandlers) uploadInitHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", "")
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

	authUserWithPlan, ok := parseAuthorizedUserWithPlan(r)
	if !ok {
		log.Errorw("failed to retrieve user data")
		pkg.WriteJSONResponse(w, http.StatusForbidden, "forbidden", "")
		return
	}

	// Enforce per-file size limit
	if authUserWithPlan.UserPlan.MaxFileSizeBytes > 0 && req.Size > authUserWithPlan.UserPlan.MaxFileSizeBytes {
		log.Warnw("plan limit: file too large",
			"user", authUserWithPlan.AuthorizedUserInfo.UserID,
			"plan", authUserWithPlan.UserPlan.PlanName,
			"limit", authUserWithPlan.UserPlan.MaxFileSizeBytes,
			"file", req.Filename,
			"size", req.Size,
		)
		pkg.WriteJSONResponse(w, http.StatusRequestEntityTooLarge, "file_too_large", map[string]any{
			"msg":                 "File exceeds the maximum allowed size for your plan",
			"max_file_size_bytes": authUserWithPlan.UserPlan.MaxFileSizeBytes,
		})
		return
	}

	userUUID := uuid.MustParse(authUserWithPlan.AuthorizedUserInfo.UserID)
	// enforce plan limits on uploads
	if exceedInfo, err := s.validateClassicUploadPlan(r.Context(), userUUID, authUserWithPlan.UserPlan); err != nil {
		if errors.Is(err, ErrFileLimitExceeded) || errors.Is(err, ErrStorageQuotaExceeded) || errors.Is(err, ErrDailyUploadLimitExceeded) {
			pkg.WriteJSONResponse(w, http.StatusTooManyRequests, "exceeded_plan_limits", exceedInfo)
		} else {
			log.Errorw("upload quota check failed", "user", userUUID, "error", err)
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		}
		return
	}

	// CreateUploadIdParams object
	params := &filedata.CreateUploadIdParams{
		OwnerUserID: uuid.MustParse(authUserWithPlan.UserID),
		FileName:    req.Filename,
		Folder:      req.Folder,
		ContentType: req.ContentType,
		Size:        req.Size,
	}
	uploadResponse, err := s.files.CreateUploadWithId(r.Context(), params)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "failed_creating_upload_id", "")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "created_upload_id", map[string]any{
		"upload_id":  uploadResponse.UploadId,
		"chunk_size": uploadResponse.ChunkSize,
	})
}

func (s *CoreHandlers) uploadPartHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	uploadId := chi.URLParam(r, "upload_id")
	partIdStr := chi.URLParam(r, "part_id")

	partId, err := strconv.Atoi(partIdStr)
	if err != nil || partId <= 0 {
		log.Errorw("invalid part number", "part_id", partId)
		pkg.WriteJSONResponse(
			w,
			http.StatusBadRequest,
			"invalid_part_number",
			"",
		)
		return
	}

	result, err := s.files.UploadPart(
		r.Context(),
		uploadId,
		int32(partId),
		r.Body,
		r.ContentLength,
	)
	if err != nil {
		pkg.WriteJSONResponse(
			w,
			http.StatusBadRequest,
			"error_uploading_part",
			"",
		)
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "uploaded_chunk", result)
}

func (s *CoreHandlers) completeUploadHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", "")
		return
	}

	uploadId := chi.URLParam(r, "upload_id")
	if strings.TrimSpace(uploadId) == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_upload_id", "")
		return
	}

	result, err := s.files.CompleteUpload(r.Context(), uploadId)
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
			log.Errorw("error completing multipart upload", "upload_id", uploadId, "error", err)
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "error_completing_upload", "")
		}
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "upload_completed", result)
}

// uploadHandler uploads a file to the user's personal storage.
// @Summary Upload a file
// @Description Uploads a file to the current user's personal storage and optionally stores it in a folder.
// @Tags Files
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "File to upload"
// @Param folder formData string false "Destination folder"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Router /api/files/upload [post]
func (s *CoreHandlers) uploadHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", "")
		return
	}

	authUserWithPlan, ok := parseAuthorizedUserWithPlan(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "failed_to_retrieve_user_data", "")
		return
	}

	authUser := authUserWithPlan.AuthorizedUserInfo
	userPlan := authUserWithPlan.UserPlan
	userUUID := authUser.UserID

	// Get file from request
	file, header, err := r.FormFile("file")
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "failed_parsing_files", "")
		log.Errorw("failed to parse form file", "error", err)
		return
	}
	defer file.Close()

	// Enforce per-file size limit
	if userPlan.MaxFileSizeBytes > 0 && header.Size > userPlan.MaxFileSizeBytes {
		log.Warnw("plan limit: file too large",
			"user", userUUID,
			"plan", userPlan.PlanName,
			"limit", userPlan.MaxFileSizeBytes,
			"file", header.Filename,
			"size", header.Size,
		)
		pkg.WriteJSONResponse(w, http.StatusRequestEntityTooLarge, "file_too_large", map[string]any{
			"msg":                 "File exceeds the maximum allowed size for your plan",
			"max_file_size_bytes": userPlan.MaxFileSizeBytes,
		})
		return
	}

	// Get folder from request if provided
	folder := r.FormValue("folder")

	// Create fileData object
	fileData := filedata.NewFileData(file, header, folder)
	if fileData == nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "invalid_file_data", "")
		return
	}
	fileData.OwnerUserID = uuid.MustParse(userUUID)

	if exceedInfo, err := s.validateClassicUploadPlan(r.Context(), uuid.MustParse(userUUID), userPlan); err != nil {
		if errors.Is(err, ErrFileLimitExceeded) || errors.Is(err, ErrStorageQuotaExceeded) || errors.Is(err, ErrDailyUploadLimitExceeded) {
			pkg.WriteJSONResponse(w, http.StatusTooManyRequests, "exceeded_plan_limits", exceedInfo)
		} else {
			log.Errorw("upload quota check failed", "user", userUUID, "error", err)
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		}
		return
	}

	if err := s.files.Upload(r.Context(), fileData); err != nil {
		switch {
		case errors.Is(err, storagetypes.ErrFileAlreadyExists):
			pkg.WriteJSONResponse(w, http.StatusConflict, "File already exists", "")
		case errors.Is(err, storagetypes.ErrStorageUnavailable):
			pkg.WriteJSONResponse(w, http.StatusServiceUnavailable, "Storage unreachable", "")
		default:
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "Upload failed", "")
		}
		return
	}

	msg := fmt.Sprintf("Files uploaded successfully: %+v\n", fileData.RequestHeaders.Filename)

	pkg.WriteJSONResponse(w, http.StatusOK, "", msg)
}
