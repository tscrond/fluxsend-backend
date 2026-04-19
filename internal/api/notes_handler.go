package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"github.com/tscrond/fluxsend-backend/internal/userdata"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

type NoteContent struct {
	Content string `json:"content"`
}

func (s *APIServer) fileNotesHandler(w http.ResponseWriter, r *http.Request) {

	switch r.Method {
	case http.MethodPut:
		s.editFileNotes(w, r)
	case http.MethodGet:
		s.getFileNotes(w, r)
	}
}

func (s *APIServer) editFileNotes(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()
	if r.Method != http.MethodPut {
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

	md5Checksum := chi.URLParam(r, "checksum")

	var req NoteContent
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "no_content", "")
		return
	}

	sanitizedNoteContent := s.backendConfig.HTMLSanitizationPolicy.Sanitize(req.Content)

	id, err := s.repository.Queries.GetFileFromChecksum(ctx, md5Checksum)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "cannot_get_file", "")
		return
	}

	log.Println("Sanitized note content: ", sanitizedNoteContent)

	if utf8.RuneCountInString(sanitizedNoteContent) > 500 {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "too_many_characters", "")
		return
	}

	parsedUUID, _ := uuid.Parse(authUserData.InternalID)
	if _, err = s.repository.Queries.UpdateNoteForFile(
		ctx,
		sqlc.UpdateNoteForFileParams{
			UserID:  parsedUUID,
			FileID:  sql.NullInt32{Valid: true, Int32: id},
			Content: sanitizedNoteContent,
		},
	); err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "cannot_update_resource", "")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "created_note", map[string]any{
		"note": sanitizedNoteContent,
	})
}

func (s *APIServer) getFileNotes(w http.ResponseWriter, r *http.Request) {
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
		w.WriteHeader(http.StatusForbidden)
		pkg.WriteJSONResponse(w, http.StatusForbidden, "authorization_failed", "")
		return
	}

	md5Checksum := chi.URLParam(r, "checksum")
	if md5Checksum == "" {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "checksum_empty", "")
		return
	}

	fileId, err := s.repository.Queries.GetFileFromChecksum(ctx, md5Checksum)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "error_query_file_id", "")
		return
	}

	parsedUUIDGet, err := uuid.Parse(authUserData.InternalID)
	if err != nil {
		log.Println("cannot parse authorized user internal id")
		w.WriteHeader(http.StatusForbidden)
		pkg.WriteJSONResponse(w, http.StatusForbidden, "authorization_failed", "")
		return
	}
	note, err := s.repository.Queries.GetNoteForFileById(ctx, sqlc.GetNoteForFileByIdParams{
		UserID: parsedUUIDGet,
		FileID: sql.NullInt32{Valid: true, Int32: fileId},
	})
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "error_get_note", "")
		return
	}
	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"content": note.Content,
	})
}
