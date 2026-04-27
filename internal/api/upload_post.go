package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"
	storagetypes "github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	"github.com/tscrond/fluxsend-backend/internal/filedata"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

func (s *APIServer) uploadHandler(w http.ResponseWriter, r *http.Request) {
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
		log.Println(err)
		return
	}
	defer file.Close()

	// Enforce per-file size limit
	if userPlan.MaxFileSizeBytes > 0 && header.Size > userPlan.MaxFileSizeBytes {
		log.Printf("[plan-limit] user=%s plan=%s limit=max_file_size_bytes value=%d file=%q size=%d: file too large", userUUID, userPlan.PlanName, userPlan.MaxFileSizeBytes, header.Filename, header.Size)
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

	if exceedInfo, err := s.validatePlan(r.Context(), uuid.MustParse(userUUID), userPlan); err != nil {
		if errors.Is(err, ErrFileLimitExceeded) || errors.Is(err, ErrStorageQuotaExceeded) || errors.Is(err, ErrDailyUploadLimitExceeded) {
			pkg.WriteJSONResponse(w, http.StatusTooManyRequests, "exceeded_plan_limits", exceedInfo)
		} else {
			log.Printf("[plan-limit] upload quota check failed for user=%s: %v", userUUID, err)
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
