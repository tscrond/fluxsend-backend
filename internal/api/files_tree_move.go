package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/logger"
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
	if ok {
		return uwp, true
	}

	cliUserWithPlan, ok := r.Context().Value(userdata.AuthorizedCLIUserWithPlanContextKey).(*userdata.AuthorizedCLIUserWithPlan)
	if !ok {
		return nil, false
	}

	return &userdata.AuthorizedUserWithPlan{
		AuthorizedUserInfo: userdata.AuthorizedUserInfo{
			InternalID: cliUserWithPlan.InternalID,
			Email:      cliUserWithPlan.Email,
			Name:       cliUserWithPlan.Name,
		},
		UserPlan: cliUserWithPlan.UserPlan,
	}, true
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

// getFilesTree returns the personal file tree for the supplied path.
// @Summary Get file tree
// @Description Returns the authenticated user's personal file tree for a given path prefix.
// @Tags Files
// @Param path query string false "Path prefix"
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Router /api/files/tree [get]
func (s *CoreHandlers) getFilesTree(w http.ResponseWriter, r *http.Request) {
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

func (s *CoreHandlers) foldersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getFolders(w, r)
	case http.MethodDelete:
		s.deleteFolder(w, r)
	default:
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", nil)
	}
}

// getFolders lists folders at the supplied path.
// @Summary List folders
// @Description Returns the folders that exist at the supplied personal path.
// @Tags Files
// @Param path query string false "Path prefix"
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Router /api/folders [get]
func (s *CoreHandlers) getFolders(w http.ResponseWriter, r *http.Request) {
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

// deleteFolder removes a folder, optionally recursively.
// @Summary Delete folder
// @Description Deletes a personal folder and optionally all nested files and folders when recursive is true.
// @Tags Files
// @Param path query string true "Folder path"
// @Param recursive query boolean false "Delete recursively"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Router /api/folders [delete]
func (s *CoreHandlers) deleteFolder(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
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
		log.Errorw("deleteFolder error", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "delete_folder_error", nil)
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"folder_deleted": folderPath,
		"files_deleted":  deletedCount,
	})
}

// moveFile relocates a file within the user's personal storage.
// @Summary Move file
// @Description Moves a personal file from one path to another.
// @Tags Files
// @Accept json
// @Produce json
// @Param request body object true "Move request"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Router /api/files/move [post]
func (s *CoreHandlers) moveFile(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
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
		log.Errorw("moveFile error", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "move_file_error", nil)
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"source":      source,
		"destination": destination,
	})
}

// moveFolder relocates a folder within the user's personal storage.
// @Summary Move folder
// @Description Moves a personal folder to another path.
// @Tags Files
// @Accept json
// @Produce json
// @Param request body object true "Move request"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Router /api/folders/move [post]
func (s *CoreHandlers) moveFolder(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
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
		log.Errorw("moveFolder error", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "move_folder_error", nil)
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"source":      source,
		"destination": destination,
		"files_moved": moved,
	})
}
