package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	storagetypes "github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	"github.com/tscrond/fluxsend-backend/internal/filedata"
	"github.com/tscrond/fluxsend-backend/internal/logger"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

const (
	defaultUploadChunkSize int64 = 50 * 50 * 1024
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
			"user", authUserWithPlan.AuthorizedUserInfo.InternalID,
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

	// CreateUploadIdParams object
	params := &filedata.CreateUploadIdParams{
		OwnerID:     uuid.MustParse(authUserWithPlan.InternalID),
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
		"upload_id": uploadResponse.UploadId,
		"chunkSize": func(chunkSize *int64) int64 {
			if chunkSize != nil {
				return *chunkSize
			}
			return defaultUploadChunkSize
		}(uploadResponse.ChunkSize),
	})
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
	userUUID := authUser.InternalID

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
	fileData.OwnerID = uuid.MustParse(userUUID)
	fileData.OwnerInternalID = authUser.InternalID

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
