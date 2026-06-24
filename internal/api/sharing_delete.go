package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/tscrond/fluxsend-backend/internal/logger"
	"github.com/tscrond/fluxsend-backend/internal/service"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

// revokeShare revokes a share token owned by the authenticated user.
// @Summary Revoke share
// @Description Revokes a previously created public share token.
// @Tags Sharing
// @Param token path string true "Share token"
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /api/share/revoke/{token} [post]
func (s *CoreHandlers) revokeShare(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
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
		log.Errorw("error revoking share", "token", token, "user", authUser.Email, "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "error_revoking_share")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "share revoked", map[string]string{
		"token_revoked": token,
	})
}
