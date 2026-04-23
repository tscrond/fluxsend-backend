package middleware

import (
	"context"
	"net/http"

	"github.com/tscrond/fluxsend-backend/internal/userdata"
	"github.com/tscrond/fluxsend-backend/pkg"
)

func EnforceFileUploadLimits(next http.Handler) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			authorizedUser, ok := r.Context().Value(userdata.AuthorizedUserContextKey).(*userdata.AuthorizedUserInfo)
			if !ok {
				pkg.WriteJSONResponse(w, http.StatusForbidden, "failed_to_retrieve_user_data", "")
				return
			}

			ctx := context.WithValue(r.Context(), userdata.AuthorizedUserContextKey, authorizedUser)

			next.ServeHTTP(w, r.WithContext(ctx))
		},
	)
}
