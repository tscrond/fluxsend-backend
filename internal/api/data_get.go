package api

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/mappings"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"github.com/tscrond/fluxsend-backend/internal/userdata"
	"github.com/tscrond/fluxsend-backend/pkg"
)

func (s *APIServer) getUserData(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "bad_request")
		return
	}

	userData, ok := r.Context().Value(userdata.AuthorizedUserContextKey).(*userdata.AuthorizedUserInfo)
	// fmt.Println(userData)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "Access Denied", map[string]any{
			"user_data": nil,
		})
		return
	}

	response := map[string]any{
		"user_data": userData,
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", response)
}

func (s *APIServer) getUserBucketData(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "bad_request")
		return
	}

	userData, ok := r.Context().Value(userdata.AuthorizedUserContextKey).(*userdata.AuthorizedUserInfo)
	// fmt.Println(userData)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "access_denied", map[string]any{
			"user_data": nil,
		})
		return
	}

	parsedUUID, err := uuid.Parse(userData.InternalID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", map[string]any{
			"bucket_data": nil,
		})
		return
	}

	filesByOwner, err := s.repository.Queries.GetFilesByOwner(ctx, parsedUUID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", map[string]any{
			"bucket_data": nil,
		})
		return
	}

	storedBucketName, err := s.repository.Queries.GetUserBucketById(ctx, parsedUUID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", map[string]any{
			"bucket_data": nil,
		})
		return
	}

	bucketName := storedBucketName.String
	if !storedBucketName.Valid || bucketName == "" {
		bucketName = pkg.GetUserBucketName(s.bucketHandler.GetBucketBaseName(), userData.InternalID)
	}

	objects := make([]mappings.ObjectMedatata, 0, len(filesByOwner))
	for _, f := range filesByOwner {
		objects = append(objects, mappings.ObjectMedatata{
			Name:        f.FileName,
			ContentType: f.FileType.String,
			Created:     time.Time{},
			Deleted:     time.Time{},
			Updated:     time.Time{},
			MD5:         f.Md5Checksum,
			Size:        f.Size.Int64,
			MediaLink:   "",
			Bucket:      bucketName,
		})
	}

	bucketData := &mappings.BucketData{
		BucketName:   bucketName,
		StorageClass: "STANDARD",
		TimeCreated:  time.Time{},
		Labels:       nil,
		Objects:      objects,
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "bucket_data_retrieved", map[string]any{
		"bucket_data": bucketData,
	})
}

func (s *APIServer) getUserPrivateFileByName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", "")
		return
	}

	ctx := r.Context()

	userData, ok := ctx.Value(userdata.AuthorizedUserContextKey).(*userdata.AuthorizedUserInfo)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "access_denied", "")
		return
	}

	fileName := r.URL.Query().Get("file")

	parsedUUID, err := uuid.Parse(userData.InternalID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "access_denied", "")
		return
	}
	downloadToken, err := s.repository.Queries.GetPrivateDownloadTokenByFileName(ctx, sqlc.GetPrivateDownloadTokenByFileNameParams{
		FileName: fileName,
		OwnerID:  parsedUUID,
	})
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"private_download_token": downloadToken.String,
	})
}
