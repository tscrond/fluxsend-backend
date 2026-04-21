package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"github.com/tscrond/fluxsend-backend/internal/userdata"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

func (s *APIServer) deleteFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodDelete {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
		return
	}

	object := r.URL.Query().Get("file")
	if object == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
		return
	}

	// parse data of logged in user
	authorizedUserData := ctx.Value(userdata.AuthorizedUserContextKey)
	authUserData, ok := authorizedUserData.(*userdata.AuthorizedUserInfo)
	if !ok {
		log.Println("cannot read authorized user data")
		pkg.WriteJSONResponse(w, http.StatusForbidden, "authorization_failed", nil)
		return
	}

	parsedUUID, err := uuid.Parse(authUserData.InternalID)
	if err != nil {
		log.Println("cannot parse authorized user internal ID: ", err)
		pkg.WriteJSONResponse(w, http.StatusForbidden, "authorization_failed", nil)
		return
	}

	bucket, err := s.resolveUserBucketName(ctx, parsedUUID, authUserData.InternalID)
	if err != nil {
		log.Println("cannot resolve user bucket name: ", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	fileRow, err := s.repository.Queries.GetFileByOwnerAndName(ctx, sqlc.GetFileByOwnerAndNameParams{
		OwnerID:  parsedUUID,
		FileName: object,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "file_not_found", nil)
			return
		}
		log.Println("error fetching file by owner and name: ", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	// dont fail if object does not exist, just report the error
	if err := s.bucketHandler.DeleteObjectFromBucket(ctx, fileRow.StorageMapping.String(), bucket); err != nil {
		log.Println("issues deleting object: ", err)
	}

	if err := s.repository.Queries.DeleteFileByNameAndId(ctx, sqlc.DeleteFileByNameAndIdParams{
		OwnerID:  parsedUUID,
		FileName: object,
	}); err != nil {
		log.Println("errors deleting file from DB: ", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "delete_file_error", nil)
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "success", map[string]any{
		"file_deleted": object,
	})
}

func (s *APIServer) deleteFilesBatch(w http.ResponseWriter, r *http.Request) {
	type ObjectsToDelete struct {
		Files []string `json:"files"`
	}

	var objToDelete ObjectsToDelete

	ctx := r.Context()

	if r.Method != http.MethodDelete {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
		return
	}

	if err := json.NewDecoder(r.Body).Decode(&objToDelete); err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
		return
	}

	if len(objToDelete.Files) == 0 {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
		return
	}

	// parse data of logged in user
	authorizedUserData := ctx.Value(userdata.AuthorizedUserContextKey)
	authUserData, ok := authorizedUserData.(*userdata.AuthorizedUserInfo)
	if !ok {
		log.Println("cannot read authorized user data")
		pkg.WriteJSONResponse(w, http.StatusForbidden, "authorization_failed", nil)
		return
	}

	parsedUUIDBatch, err := uuid.Parse(authUserData.InternalID)
	if err != nil {
		log.Printf("invalid authorized user internal ID %q: %v", authUserData.InternalID, err)
		pkg.WriteJSONResponse(w, http.StatusForbidden, "authorization_failed", nil)
		return
	}

	bucket, err := s.resolveUserBucketName(ctx, parsedUUIDBatch, authUserData.InternalID)
	if err != nil {
		log.Println("cannot resolve user bucket name: ", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	type resolvedDelete struct {
		logicalName string
		objectKey   string
	}

	resolvedDeletes := make([]resolvedDelete, 0, len(objToDelete.Files))
	storageObjectKeys := make([]string, 0, len(objToDelete.Files))
	failedFiles := make([]string, 0)

	for _, object := range objToDelete.Files {
		if object == "" {
			continue
		}

		fileRow, lookupErr := s.repository.Queries.GetFileByOwnerAndName(ctx, sqlc.GetFileByOwnerAndNameParams{
			OwnerID:  parsedUUIDBatch,
			FileName: object,
		})
		if lookupErr != nil {
			log.Printf("errors resolving file from DB (%s): %v", object, lookupErr)
			failedFiles = append(failedFiles, object)
			continue
		}

		resolvedDeletes = append(resolvedDeletes, resolvedDelete{
			logicalName: object,
			objectKey:   fileRow.StorageMapping.String(),
		})
		storageObjectKeys = append(storageObjectKeys, fileRow.StorageMapping.String())
	}

	// dont fail if object does not exist, just report the error
	if err := s.bucketHandler.DeleteObjectsFromBucket(ctx, storageObjectKeys, bucket); err != nil {
		log.Println("issues deleting object(s): ", err)
	}

	deletedFiles := make([]string, 0, len(resolvedDeletes))
	for _, resolved := range resolvedDeletes {
		err := s.repository.Queries.DeleteFileByNameAndId(ctx, sqlc.DeleteFileByNameAndIdParams{
			OwnerID:  parsedUUIDBatch,
			FileName: resolved.logicalName,
		})
		if err != nil {
			log.Printf("errors deleting file from DB (%s): %v", resolved.logicalName, err)
			failedFiles = append(failedFiles, resolved.logicalName)
			continue
		}

		deletedFiles = append(deletedFiles, resolved.logicalName)
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "success", map[string]any{
		"files_deleted": deletedFiles,
		"files_failed":  failedFiles,
	})
}

func (s *APIServer) resolveUserBucketName(ctx context.Context, userUUID uuid.UUID, internalID string) (string, error) {
	storedBucketName, err := s.repository.Queries.GetUserBucketById(ctx, userUUID)
	if err != nil {
		return "", err
	}

	bucketName := strings.TrimSpace(storedBucketName.String)
	if storedBucketName.Valid && bucketName != "" {
		return bucketName, nil
	}

	return fmt.Sprintf("%s-%s", s.bucketHandler.GetBucketBaseName(), internalID), nil
}

func (s *APIServer) deleteAccount(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodDelete {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
		return
	}

	// parse data of logged in user
	authorizedUserData := ctx.Value(userdata.AuthorizedUserContextKey)
	authUserData, ok := authorizedUserData.(*userdata.AuthorizedUserInfo)
	if !ok {
		log.Println("cannot read authorized user data")
		pkg.WriteJSONResponse(w, http.StatusForbidden, "authorization_failed", nil)
		return
	}
	bucketName := pkg.GetUserBucketName(s.bucketHandler.GetBucketBaseName(), authUserData.InternalID)

	fullResponse := map[string]any{}
	fullResponse["bucket"] = map[string]any{
		"name":    bucketName,
		"deleted": false,
	}

	type DeleteAccountRequest struct {
		DeleteUserData bool `json:"delete_user_data"`
	}

	var req DeleteAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", "")
		return
	}

	if req.DeleteUserData {
		if err := s.bucketHandler.DeleteBucket(ctx, bucketName); err != nil {
			log.Printf("failed to delete bucket %s err: %s\n", bucketName, err)
			fullResponse["bucket"] = map[string]any{
				"name":    bucketName,
				"deleted": false,
			}
		}

		fullResponse["bucket"] = map[string]any{
			"name":    bucketName,
			"deleted": true,
		}
	}

	parsedUUIDDel, err := uuid.Parse(authUserData.InternalID)
	if err != nil {
		log.Println("failed parsing user ID")
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "user_id_parse_error", nil)
		return
	}
	deletedAccount, err := s.repository.Queries.DeleteAccount(ctx, parsedUUIDDel)
	if err != nil {
		log.Println("issues deleting object: ", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "delete_account_failure", nil)
		return
	}

	fullResponse["account_deleted"] = map[string]any{
		"id":        deletedAccount.ID.String(),
		"email":     deletedAccount.UserEmail,
		"user_name": deletedAccount.UserName.String,
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "success", fullResponse)

}
