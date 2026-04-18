package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"github.com/tscrond/fluxsend-backend/internal/userdata"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

type fileTreeEntry struct {
	Name        string `json:"name"`
	FileType    string `json:"file_type"`
	Size        int64  `json:"size"`
	MD5Checksum string `json:"md5_checksum"`
}

type moveRequest struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

func normalizePath(path string) string {
	trimmed := strings.TrimSpace(path)
	return strings.Trim(trimmed, "/")
}

func relativeToPath(fullPath, currentPath string) (string, bool) {
	if currentPath == "" {
		return fullPath, true
	}
	prefix := currentPath + "/"
	if strings.HasPrefix(fullPath, prefix) {
		return strings.TrimPrefix(fullPath, prefix), true
	}
	return "", false
}

func folderPrefix(path string) string {
	if path == "" {
		return ""
	}
	return path + "/"
}

func parseAuthorizedUser(r *http.Request) (*userdata.AuthorizedUserInfo, bool) {
	authorizedUserData := r.Context().Value(userdata.AuthorizedUserContextKey)
	authUserData, ok := authorizedUserData.(*userdata.AuthorizedUserInfo)
	if !ok {
		return nil, false
	}
	return authUserData, true
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
	filesByOwner, err := s.repository.Queries.GetFilesByOwner(r.Context(), uuid.NullUUID{Valid: true, UUID: userUUID})
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	foldersSet := map[string]struct{}{}
	treeFiles := make([]fileTreeEntry, 0)

	for _, f := range filesByOwner {
		rel, include := relativeToPath(f.FileName, path)
		if !include || rel == "" {
			continue
		}

		if slash := strings.Index(rel, "/"); slash >= 0 {
			foldersSet[rel[:slash]] = struct{}{}
			continue
		}

		treeFiles = append(treeFiles, fileTreeEntry{
			Name:        f.FileName,
			FileType:    f.FileType.String,
			Size:        f.Size.Int64,
			MD5Checksum: f.Md5Checksum,
		})
	}

	folders := make([]string, 0, len(foldersSet))
	for folder := range foldersSet {
		folders = append(folders, folder)
	}
	sort.Strings(folders)
	sort.Slice(treeFiles, func(i, j int) bool {
		return treeFiles[i].Name < treeFiles[j].Name
	})

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"path":    path,
		"folders": folders,
		"files":   treeFiles,
	})
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
	filesByOwner, err := s.repository.Queries.GetFilesByOwner(r.Context(), uuid.NullUUID{Valid: true, UUID: userUUID})
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	foldersSet := map[string]struct{}{}
	for _, f := range filesByOwner {
		rel, include := relativeToPath(f.FileName, path)
		if !include || rel == "" {
			continue
		}
		if slash := strings.Index(rel, "/"); slash >= 0 {
			foldersSet[rel[:slash]] = struct{}{}
		}
	}

	folders := make([]string, 0, len(foldersSet))
	for folder := range foldersSet {
		folders = append(folders, folder)
	}
	sort.Strings(folders)

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

	filesByOwner, err := s.repository.Queries.GetFilesByOwner(r.Context(), uuid.NullUUID{Valid: true, UUID: userUUID})
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	prefix := folderPrefix(folderPath)
	toDelete := make([]sqlc.File, 0)
	for _, f := range filesByOwner {
		if strings.HasPrefix(f.FileName, prefix) {
			toDelete = append(toDelete, f)
		}
	}

	if len(toDelete) > 0 && !recursive {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "recursive_required", nil)
		return
	}

	bucket := pkg.GetUserBucketName(s.bucketHandler.GetBucketBaseName(), authUserData.InternalID)
	deletedCount := 0
	for _, f := range toDelete {
		if err := s.bucketHandler.DeleteObjectFromBucket(r.Context(), f.FileName, bucket); err != nil {
			log.Println("failed deleting object from bucket:", err)
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "delete_folder_error", nil)
			return
		}
		if err := s.repository.Queries.DeleteFileByNameAndId(r.Context(), sqlc.DeleteFileByNameAndIdParams{
			OwnerID:  uuid.NullUUID{Valid: true, UUID: userUUID},
			FileName: f.FileName,
		}); err != nil {
			log.Println("failed deleting file metadata:", err)
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "delete_folder_error", nil)
			return
		}
		deletedCount++
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

	authUserData, userUUID, ok := parseAuthorizedUserUUID(r)
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

	_, err := s.repository.Queries.GetFileByOwnerAndName(r.Context(), sqlc.GetFileByOwnerAndNameParams{
		OwnerID:  uuid.NullUUID{Valid: true, UUID: userUUID},
		FileName: source,
	})
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusNotFound, "source_not_found", nil)
		return
	}

	bucket := pkg.GetUserBucketName(s.bucketHandler.GetBucketBaseName(), authUserData.InternalID)
	if err := s.bucketHandler.MoveObjectInBucket(r.Context(), source, destination, bucket); err != nil {
		log.Println("failed moving object in bucket:", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "move_file_error", nil)
		return
	}

	if err := s.repository.Queries.UpdateFileNameByOwnerAndName(r.Context(), sqlc.UpdateFileNameByOwnerAndNameParams{
		FileName:   destination,
		OwnerID:    uuid.NullUUID{Valid: true, UUID: userUUID},
		FileName_2: source,
	}); err != nil {
		log.Println("failed updating file metadata:", err)
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

	authUserData, userUUID, ok := parseAuthorizedUserUUID(r)
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

	filesByOwner, err := s.repository.Queries.GetFilesByOwner(r.Context(), uuid.NullUUID{Valid: true, UUID: userUUID})
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	sourcePrefix := folderPrefix(source)
	toMove := make([]sqlc.File, 0)
	for _, f := range filesByOwner {
		if strings.HasPrefix(f.FileName, sourcePrefix) {
			toMove = append(toMove, f)
		}
	}

	if len(toMove) == 0 {
		pkg.WriteJSONResponse(w, http.StatusNotFound, "source_not_found", nil)
		return
	}

	bucket := pkg.GetUserBucketName(s.bucketHandler.GetBucketBaseName(), authUserData.InternalID)
	moved := 0
	for _, f := range toMove {
		relative := strings.TrimPrefix(f.FileName, sourcePrefix)
		newPath := destination + "/" + relative

		if err := s.bucketHandler.MoveObjectInBucket(r.Context(), f.FileName, newPath, bucket); err != nil {
			log.Println("failed moving object in bucket:", err)
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "move_folder_error", nil)
			return
		}

		if err := s.repository.Queries.UpdateFileNameByOwnerAndName(r.Context(), sqlc.UpdateFileNameByOwnerAndNameParams{
			FileName:   newPath,
			OwnerID:    uuid.NullUUID{Valid: true, UUID: userUUID},
			FileName_2: f.FileName,
		}); err != nil {
			log.Println("failed updating file metadata:", err)
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "move_folder_error", nil)
			return
		}

		moved++
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"source":      source,
		"destination": destination,
		"files_moved": moved,
	})
}
