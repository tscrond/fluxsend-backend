package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	storagetypes "github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	"github.com/tscrond/fluxsend-backend/internal/filedata"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

func (s *APIServer) uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", "")
		return
	}

	authUser, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "failed_to_retrieve_user_data", "")
		return
	}

	// Get file from request
	file, header, err := r.FormFile("file")
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "failed_parsing_files", "")
		log.Println(err)
		return
	}
	defer file.Close()

	// Get folder from request if provided
	folder := r.FormValue("folder")

	// Create fileData object
	fileData := filedata.NewFileData(file, header, folder)
	if fileData == nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "invalid_file_data", "")
		return
	}
	fileData.OwnerID = userUUID
	fileData.OwnerInternalID = authUser.InternalID

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
