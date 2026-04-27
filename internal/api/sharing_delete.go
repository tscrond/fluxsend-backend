package api

import (
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/tscrond/fluxsend-backend/internal/service"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

func (s *APIServer) revokeShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "bad_request")
		return
	}

	authUser, ok := parseAuthorizedUser(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	token := chi.URLParam(r, "token")
	if token == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "empty_token")
		return
	}

	if err := s.shares.RevokeShare(r.Context(), token, authUser.Email); err != nil {
		if errors.Is(err, service.ErrShareNotFound) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "", "share_not_found")
			return
		}
		log.Printf("error revoking share %q for user %q: %v\n", token, authUser.Email, err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "error_revoking_share")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "share revoked", map[string]string{
		"token_revoked": token,
	})
}
