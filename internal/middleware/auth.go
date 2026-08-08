package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func WithOAuthProvider(provider string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			routeCtx := chi.RouteContext(r.Context())
			routeCtx.URLParams.Add("provider", provider)

			next.ServeHTTP(w, r.WithContext(r.Context()))
		})
	}
}
