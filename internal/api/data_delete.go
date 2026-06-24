package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

// deleteFile removes a personal file by path.
// @Summary Delete a file
// @Description Deletes a file from the authenticated user's personal storage.
// @Tags Files
// @Param file query string true "File path"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /api/files/delete [delete]
func (s *CoreHandlers) deleteFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
		return
	}

	object := r.URL.Query().Get("file")
	if object == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
		return
	}

	authUser, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "authorization_failed", nil)
		return
	}

	if err := s.files.DeleteFile(r.Context(), userUUID, authUser.InternalID, object); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "file_not_found", nil)
			return
		}
		log.Println("error deleting file:", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "success", map[string]any{
		"file_deleted": object,
	})
}

// deleteFilesBatch removes multiple personal files in one request.
// @Summary Delete multiple files
// @Description Deletes several personal files in one request.
// @Tags Files
// @Accept json
// @Produce json
// @Param request body object true "Files to delete"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Router /api/files/delete/batch [delete]
func (s *CoreHandlers) deleteFilesBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
		return
	}

	type ObjectsToDelete struct {
		Files []string `json:"files"`
	}
	var objToDelete ObjectsToDelete
	if err := json.NewDecoder(r.Body).Decode(&objToDelete); err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
		return
	}
	if len(objToDelete.Files) == 0 {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
		return
	}

	authUser, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "authorization_failed", nil)
		return
	}

	deleted, failed, err := s.files.DeleteFiles(r.Context(), userUUID, authUser.InternalID, objToDelete.Files)
	if err != nil {
		log.Println("batch delete error:", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "success", map[string]any{
		"files_deleted": deleted,
		"files_failed":  failed,
	})
}

// deleteAccount removes the authenticated user's account and optional data.
// @Summary Delete account
// @Description Deletes the authenticated user's account and optionally keeps or removes bucket data.
// @Tags User
// @Accept json
// @Produce json
// @Param request body object true "Delete account request"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Router /api/user/account/delete [delete]
func (s *CoreHandlers) deleteAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
		return
	}

	authUser, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "authorization_failed", nil)
		return
	}

	type DeleteAccountRequest struct {
		DeleteUserData bool `json:"delete_user_data"`
	}
	var req DeleteAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
		return
	}

	result, err := s.users.DeleteAccount(r.Context(), userUUID, authUser.InternalID, authUser.Name, req.DeleteUserData)
	if err != nil {
		log.Println("error deleting account:", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "delete_account_failure", nil)
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "success", map[string]any{
		"bucket": map[string]any{
			"name":    result.BucketName,
			"deleted": result.BucketDeleted,
		},
		"account_deleted": map[string]any{
			"id":        result.AccountID,
			"email":     result.Email,
			"user_name": result.UserName,
		},
	})
}
