package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/service"
	"github.com/tscrond/fluxsend-backend/internal/userdata"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

type moveRequest struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

func normalizePath(path string) string {
	trimmed := strings.TrimSpace(path)
	return strings.Trim(trimmed, "/")
}

func parseAuthorizedUserWithPlan(r *http.Request) (*userdata.AuthorizedUserWithPlan, bool) {
	uwp, ok := r.Context().Value(userdata.AuthorizedUserWithPlanContextKey).(*userdata.AuthorizedUserWithPlan)
	if !ok {
		return nil, false
	}
	return uwp, true
}

func parseAuthorizedUser(r *http.Request) (*userdata.AuthorizedUserInfo, bool) {
	uwp, ok := parseAuthorizedUserWithPlan(r)
	if !ok {
		return nil, false
	}
	return &uwp.AuthorizedUserInfo, true
}

func parseAuthorizedUserUUID(r *http.Request) (*userdata.AuthorizedUserInfo, uuid.UUID, bool) {
	authUserData, ok := parseAuthorizedUser(r)
	if !ok {
		return nil, uuid.Nil, false
	}
	parsedUUID, err := uuid.Parse(authUserData.InternalID)
	if err != nil {
		return nil, uuid.Nil, false
	}
	return authUserData, parsedUUID, true
}

func (s *APIServer) getFilesTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
		return
	}

	_, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "authorization_failed", nil)
		return
	}

	path := normalizePath(r.URL.Query().Get("path"))
	tree, err := s.files.GetFilesTree(r.Context(), userUUID, path)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", tree)
}

func (s *APIServer) foldersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getFolders(w, r)
	case http.MethodDelete:
		s.deleteFolder(w, r)
	default:
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
	}
}

func (s *APIServer) getFolders(w http.ResponseWriter, r *http.Request) {
	_, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "authorization_failed", nil)
		return
	}

	path := normalizePath(r.URL.Query().Get("path"))
	folders, err := s.files.GetFolders(r.Context(), userUUID, path)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"path":    path,
		"folders": folders,
	})
}

func (s *APIServer) deleteFolder(w http.ResponseWriter, r *http.Request) {
	authUserData, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "authorization_failed", nil)
		return
	}

	folderPath := normalizePath(r.URL.Query().Get("path"))
	if folderPath == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
		return
	}

	recursive := strings.EqualFold(r.URL.Query().Get("recursive"), "true")

	deletedCount, err := s.files.DeleteFolder(r.Context(), userUUID, authUserData.InternalID, folderPath, recursive)
	if err != nil {
		if errors.Is(err, service.ErrRecursiveRequired) {
			pkg.WriteJSONResponse(w, http.StatusBadRequest, "recursive_required", nil)
			return
		}
		log.Println("deleteFolder error:", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "delete_folder_error", nil)
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"folder_deleted": folderPath,
		"files_deleted":  deletedCount,
	})
}

func (s *APIServer) moveFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
		return
	}

	_, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "authorization_failed", nil)
		return
	}

	var req moveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
		return
	}

	source := normalizePath(req.Source)
	destination := normalizePath(req.Destination)
	if source == "" || destination == "" || source == destination {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
		return
	}

	if err := s.files.MoveFile(r.Context(), userUUID, source, destination); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "source_not_found", nil)
			return
		}
		log.Println("moveFile error:", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "move_file_error", nil)
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"source":      source,
		"destination": destination,
	})
}

func (s *APIServer) moveFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
		return
	}

	_, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "authorization_failed", nil)
		return
	}

	var req moveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
		return
	}

	source := normalizePath(req.Source)
	destination := normalizePath(req.Destination)
	if source == "" || destination == "" || source == destination {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
		return
	}
	if strings.HasPrefix(destination+"/", source+"/") {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_destination", nil)
		return
	}

	moved, err := s.files.MoveFolder(r.Context(), userUUID, source, destination)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "source_not_found", nil)
			return
		}
		log.Println("moveFolder error:", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "move_folder_error", nil)
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"source":      source,
		"destination": destination,
		"files_moved": moved,
	})
}
