package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

func (s *APIServer) renameWorkspace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
		return
	}

	_, _, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	var body struct {
		WorkspaceID string `json:"workspace_id"`
		Name        string `json:"name"`
	}

	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_json")
		return
	}

	if body.Name == "" || body.WorkspaceID == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "workspace_id_and_name_required")
		return
	}

	workspaceID, err := uuid.Parse(body.WorkspaceID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_workspace_id")
		return
	}

	updated, err := s.workspaces.RenameWorkspace(r.Context(), workspaceID, body.Name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "", "workspace_not_found")
			return
		}
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "workspace_renamed", updated)
}
