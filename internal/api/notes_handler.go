package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/tscrond/fluxsend-backend/internal/service"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

func (s *APIServer) fileNotesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut:
		s.editFileNotes(w, r)
	case http.MethodGet:
		s.getFileNotes(w, r)
	}
}

func (s *APIServer) editFileNotes(w http.ResponseWriter, r *http.Request) {
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

	type NoteContent struct {
		Content string `json:"content"`
	}
	var req NoteContent
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "no_content", "")
		return
	}

	sanitized, err := s.files.UpsertNote(r.Context(), userUUID, checksum, req.Content)
	if err != nil {
		if errors.Is(err, service.ErrNoteTooLong) {
			pkg.WriteJSONResponse(w, http.StatusBadRequest, "too_many_characters", "")
			return
		}
		log.Println("error upserting note:", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "cannot_update_resource", "")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "created_note", map[string]any{
		"note": sanitized,
	})
}

func (s *APIServer) getFileNotes(w http.ResponseWriter, r *http.Request) {
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

	content, err := s.files.GetNote(r.Context(), userUUID, checksum)
	if err != nil {
		log.Println("error getting note:", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "error_get_note", "")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"content": content,
	})
}
