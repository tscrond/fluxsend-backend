package api

import (
	"net/http"

	"github.com/tscrond/fluxsend-backend/internal/userdata"
	"github.com/tscrond/fluxsend-backend/pkg"
)

func (s *APIServer) getUserData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "bad_request")
		return
	}

	userData, ok := r.Context().Value(userdata.AuthorizedUserContextKey).(*userdata.AuthorizedUserInfo)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "Access Denied", map[string]any{
			"user_data": nil,
		})
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"user_data": userData,
	})
}

func (s *APIServer) getUserBucketData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "bad_request")
		return
	}

	authUser, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "access_denied", map[string]any{
			"bucket_data": nil,
		})
		return
	}

	bucketData, err := s.users.GetBucketData(r.Context(), userUUID, authUser.InternalID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", map[string]any{
			"bucket_data": nil,
		})
		return
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

	_, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "access_denied", "")
		return
	}

	fileName := r.URL.Query().Get("file")

	token, err := s.users.GetPrivateDownloadToken(r.Context(), userUUID, fileName)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"private_download_token": token,
	})
}
