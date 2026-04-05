package api

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"github.com/tscrond/fluxsend-backend/internal/userdata"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

func (s *APIServer) downloadThroughProxyPersonal(w http.ResponseWriter, r *http.Request) {
	// 0*. generate the access token (include token in shares db table - /share endpoint)
	// * this step is done in /share endpoint exclusively
	// 1. take in token as a query parameter or path
	// 2. check if user is authorized
	// 3. check if token exists
	// 4. generate short-lived signed URL
	// 5. redirect user to the signed URL
	ctx := r.Context()

	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", "")
		return
	}

	// parse data of logged in user
	authorizedUserData := ctx.Value(userdata.AuthorizedUserContextKey)
	authUserData, ok := authorizedUserData.(*userdata.AuthorizedUserInfo)
	if !ok {
		log.Println("cannot read authorized user data")
		pkg.WriteJSONResponse(w, http.StatusForbidden, "authorization_failed", "")
		return
	}

	token := chi.URLParam(r, "token")

	if token == "" {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "empty_token", "")
		return
	}

	mode := r.URL.Query().Get("mode") // "inline" or "download"
	if mode != "inline" && mode != "download" && mode != "" {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "invalid_download_mode", "")
		return
	}

	// 2. check if token exists, if exists, return file ID
	_, err := s.repository.Queries.GetFileIdFromToken(ctx, sql.NullString{Valid: true, String: token})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "file_does_not_exist", "")
			return
		} else {
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "token_check_error", "")
			return
		}
	}

	bucketAndObjectRow, err := s.repository.Queries.GetBucketObjectAndOwnerFromPrivateToken(ctx, sql.NullString{Valid: true, String: token})
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "cannot_get_bucket_data", "")
		return
	}

	if authUserData.Id != bucketAndObjectRow.OwnerGoogleID.String {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "access_denied", "")
		return
	}

	bucket := bucketAndObjectRow.BucketName.String
	object := bucketAndObjectRow.ObjectName

	signedUrl, err := s.resolveDownloadURL(ctx, bucket, object, time.Now().Add(1*time.Minute))
	if err != nil {
		log.Printf("cannot generate download URL for bucket=%q object=%q: %v", bucket, object, err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "cannot_generate_url", "")
		return
	}

	http.Redirect(w, r, signedUrl, http.StatusFound)
}

func (s *APIServer) downloadThroughProxy(w http.ResponseWriter, r *http.Request) {
	// download link should look like this: https://<domain_name>/download?token=143adfsadfasd9a9sdf7a89df9
	// or: https://<domain_name>/d/143adfsadfasd9a9sdf7a89df9
	// 0*. generate the access token (include token in shares db table - /share endpoint)
	// * this step is done in /share endpoint exclusively
	// 1. take in token as a query parameter or path
	// 2. check if token exists
	// 3. generate short-lived signed URL
	// 4. redirect user to the signed URL
	ctx := r.Context()

	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", "")
		return
	}

	mode := r.URL.Query().Get("mode") // "inline" or "download"
	if mode != "inline" && mode != "download" && mode != "" {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "invalid_download_mode", "")
		return
	}

	// 1. parse sharing token from url path
	sharingToken := chi.URLParam(r, "token")

	// 1.5 check token expiration times
	expiresAt, err := s.repository.Queries.GetTokenExpirationTime(ctx, sharingToken)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "error_checking_expiration", "")
		return
	}

	if expiresAt.Before(time.Now()) {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "past_expiration_time_or_does_not_exist", "")
		return
	}

	// 2. check if token exists, if exists, return shared file ID
	_, err = s.repository.Queries.GetSharedFileIdFromToken(ctx, sharingToken)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "token_does_not_exist", "")
			return
		} else {
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "token_check_error", "")
			return
		}
	}

	// 2.5 get the bucket of shared resource + get the object name
	bucketAndObject, err := s.repository.Queries.GetBucketAndObjectFromToken(ctx, sharingToken)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "cannot_get_bucket_data", "")
		return
	}

	// 3. generate signed url
	signedUrl, err := s.resolveDownloadURL(ctx,
		bucketAndObject.UserBucket.String,
		bucketAndObject.FileName,
		time.Now().Add(time.Minute),
	)

	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "signed_url_error", "")
		return
	}

	http.Redirect(w, r, signedUrl, http.StatusFound)
}

func (s *APIServer) resolveDownloadURL(ctx context.Context, bucket, object string, expiresAt time.Time) (string, error) {
	if s.cloudFrontSigner != nil {
		return s.cloudFrontSigner.SignURL(bucket, object, expiresAt)
	}

	return s.bucketHandler.GenerateSignedURL(ctx, bucket, object, expiresAt)
}

func (s *APIServer) getDataSharedForUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodGet {
		status := http.StatusBadRequest
		pkg.WriteJSONResponse(w, status, "bad_request", "")
		return
	}

	authorizedUserData := ctx.Value(userdata.AuthorizedUserContextKey)
	authUserData, ok := authorizedUserData.(*userdata.AuthorizedUserInfo)
	if !ok {
		log.Println("cannot read authorized user data")
		return
	}

	sharedFor := sql.NullString{Valid: true, String: authUserData.Email}
	filesShared, err := s.repository.Queries.GetFilesSharedWithUser(ctx, sharedFor)
	if err != nil {
		log.Println(err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
	}

	filesSharedPrep := prepSharedFilesFormat(filesShared)

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"files": filesSharedPrep,
	})
}

func (s *APIServer) getDataSharedByUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", "")
		return
	}

	authorizedUserData := ctx.Value(userdata.AuthorizedUserContextKey)
	authUserData, ok := authorizedUserData.(*userdata.AuthorizedUserInfo)
	if !ok {
		log.Println("cannot read authorized user data")
		return
	}

	sharedFor := sql.NullString{Valid: true, String: authUserData.Email}
	filesShared, err := s.repository.Queries.GetFilesSharedByUser(ctx, sharedFor)
	if err != nil {
		log.Println(err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
	}

	filesSharedPrep := prepSharedByFilesFormat(filesShared)

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"files": filesSharedPrep,
	})
}

// helper function for sharing (converts format of table for JSON output)
func prepSharedFilesFormat(sharedFiles []sqlc.GetFilesSharedWithUserRow) []any {

	var allfiles []any
	for _, sharedFile := range sharedFiles {

		savedData := make(map[string]any)

		savedData["file_id"] = sharedFile.FileID.Int32
		savedData["owner_google_id"] = sharedFile.OwnerGoogleID.String
		savedData["file_name"] = sharedFile.FileName
		savedData["file_type"] = sharedFile.FileType.String
		savedData["md5_checksum"] = sharedFile.Md5Checksum
		savedData["shared_by"] = sharedFile.SharedBy.String
		savedData["shared_for"] = sharedFile.SharedFor.String
		savedData["sharing_token"] = sharedFile.SharingToken
		savedData["expires_at"] = sharedFile.ExpiresAt
		savedData["size"] = sharedFile.Size.Int64

		allfiles = append(allfiles, savedData)
	}

	return allfiles
}

// helper function for sharing
func prepSharedByFilesFormat(sharedFiles []sqlc.GetFilesSharedByUserRow) []any {

	var allfiles []any
	for _, sharedFile := range sharedFiles {

		savedData := make(map[string]any)

		savedData["file_id"] = sharedFile.FileID.Int32
		savedData["owner_google_id"] = sharedFile.OwnerGoogleID.String
		savedData["file_name"] = sharedFile.FileName
		savedData["file_type"] = sharedFile.FileType.String
		savedData["md5_checksum"] = sharedFile.Md5Checksum
		savedData["shared_by"] = sharedFile.SharedBy.String
		savedData["shared_for"] = sharedFile.SharedFor.String
		savedData["sharing_token"] = sharedFile.SharingToken
		savedData["expires_at"] = sharedFile.ExpiresAt
		savedData["size"] = sharedFile.Size.Int64

		allfiles = append(allfiles, savedData)
	}

	return allfiles
}
