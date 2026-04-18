package api

import (
	"net/http"

	"github.com/google/uuid"
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

	bucketData, err := s.bucketHandler.GetUserBucketData(ctx, userData.InternalID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", map[string]any{
			"bucket_data": nil,
		})
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "internal_error", map[string]any{
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
		OwnerID:  uuid.NullUUID{Valid: true, UUID: parsedUUID},
	})
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"private_download_token": downloadToken.String,
	})
}
