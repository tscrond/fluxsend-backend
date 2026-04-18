package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

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

	bucket := fmt.Sprintf("%s-%s", s.bucketHandler.GetBucketBaseName(), authUserData.InternalID)

	// dont fail if object does not exist, just report the error
	if err := s.bucketHandler.DeleteObjectFromBucket(ctx, object, bucket); err != nil {
		log.Println("issues deleting object: ", err)
	}

	parsedUUID, _ := uuid.Parse(authUserData.InternalID)
	if err := s.repository.Queries.DeleteFileByNameAndId(ctx, sqlc.DeleteFileByNameAndIdParams{
		OwnerID:  uuid.NullUUID{Valid: true, UUID: parsedUUID},
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

	bucket := fmt.Sprintf("%s-%s", s.bucketHandler.GetBucketBaseName(), authUserData.InternalID)

	// dont fail if object does not exist, just report the error
	if err := s.bucketHandler.DeleteObjectsFromBucket(ctx, objToDelete.Files, bucket); err != nil {
		log.Println("issues deleting object(s): ", err)
	}

	parsedUUIDBatch, err := uuid.Parse(authUserData.InternalID)
	if err != nil {
		log.Printf("invalid authorized user internal ID %q: %v", authUserData.InternalID, err)
		pkg.WriteJSONResponse(w, http.StatusForbidden, "authorization_failed", nil)
		return
	}
	deletedFiles := make([]string, 0, len(objToDelete.Files))
	failedFiles := make([]string, 0)

	for _, object := range objToDelete.Files {
		if object == "" {
			continue
		}

		err := s.repository.Queries.DeleteFileByNameAndId(ctx, sqlc.DeleteFileByNameAndIdParams{
			OwnerID:  uuid.NullUUID{Valid: true, UUID: parsedUUIDBatch},
			FileName: object,
		})
		if err != nil {
			log.Printf("errors deleting file from DB (%s): %v", object, err)
			failedFiles = append(failedFiles, object)
			continue
		}

		deletedFiles = append(deletedFiles, object)
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "success", map[string]any{
		"files_deleted": deletedFiles,
		"files_failed":  failedFiles,
	})
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
