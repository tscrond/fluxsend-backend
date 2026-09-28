package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/tscrond/fluxsend-backend/internal/logger"
	"github.com/tscrond/fluxsend-backend/internal/service"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

func (s *CoreHandlers) fileNotesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut:
		s.editFileNotes(w, r)
	case http.MethodGet:
		s.getFileNotes(w, r)
	}
}

// editFileNotes updates the note attached to a file.
// @Summary Edit file note
// @Description Updates or creates the note for a file identified by checksum and file name.
// @Tags Files
// @Accept json
// @Produce json
// @Param checksum path string true "File checksum"
// @Param file_name query string true "File name"
// @Param request body object true "Note content request"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Router /api/files/{checksum}/note [put]
func (s *CoreHandlers) editFileNotes(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodPut {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", "")
		return
	}

	_, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "authorization_failed", "")
		return
	}

	checksum := chi.URLParam(r, "checksum")
	if checksum == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "checksum_empty", "")
		return
	}
	fileName := r.URL.Query().Get("file_name")
	if fileName == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "file_name_empty", "")
		return
	}

	type NoteContent struct {
		Content string `json:"content"`
	}
	var req NoteContent
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "no_content", "")
		return
	}

	sanitized, err := s.files.UpsertNote(r.Context(), userUUID, checksum, fileName, req.Content)
	if err != nil {
		if errors.Is(err, service.ErrNoteTooLong) {
			pkg.WriteJSONResponse(w, http.StatusBadRequest, "too_many_characters", "")
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "file_not_found", "")
			return
		}
		log.Errorw("error upserting note", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "cannot_update_resource", "")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "created_note", map[string]any{
		"note": sanitized,
	})
}

// getFileNotes returns the note attached to a file.
// @Summary Get file note
// @Description Returns the note content for a file identified by checksum and file name.
// @Tags Files
// @Param checksum path string true "File checksum"
// @Param file_name query string true "File name"
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Router /api/files/{checksum}/note [get]
func (s *CoreHandlers) getFileNotes(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", "")
		return
	}

	_, userUUID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "authorization_failed", "")
		return
	}

	checksum := chi.URLParam(r, "checksum")
	if checksum == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "checksum_empty", "")
		return
	}
	fileName := r.URL.Query().Get("file_name")
	if fileName == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "file_name_empty", "")
		return
	}

	content, err := s.files.GetNote(r.Context(), userUUID, checksum, fileName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "file_not_found", "")
			return
		}
		log.Errorw("error getting note", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "error_get_note", "")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"content": content,
	})
}
