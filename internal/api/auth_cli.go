package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/logger"
	"github.com/tscrond/fluxsend-backend/internal/userdata"
	"github.com/tscrond/fluxsend-backend/pkg"
)

func (s *CLIServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			log := logger.FromContext(r.Context())
			var apiKeyResponse struct {
				APIKey string `json:"api_key"`
			}
			if err := json.NewDecoder(r.Body).Decode(&apiKeyResponse); err != nil {
				pkg.WriteJSONResponse(w, http.StatusForbidden, "", "Unauthorized")
				log.Errorf("error parsing api key data %s", err)
				return
			}

			if apiKeyResponse.APIKey == "" {
				pkg.WriteJSONResponse(w, http.StatusForbidden, "", "Unauthorized")
				log.Errorf("missing API key")
				return
			}

			apiKeyIdentity, err := s.repository.Queries().GetIdentityByAPIKey(r.Context(), uuid.New()) // later use: apiKeyResponse.APIKey
			if err != nil {
				log.Errorw("cannot find identity for associated API Key", "api_key", apiKeyResponse.APIKey, "error", err)
				pkg.WriteJSONResponse(w, http.StatusForbidden, "", "User identity not found")
				return
			}

			log.Infof("api key identity checked", "identity", apiKeyIdentity)

			authorizedUser := &userdata.AuthorizedCLIUserInfo{}
			userPlan := &userdata.UserPlan{}

			ctx := context.WithValue(r.Context(), userdata.AuthorizedCLIUserWithPlanContextKey, &userdata.AuthorizedCLIUserWithPlan{
				AuthorizedCLIUserInfo: *authorizedUser,
				UserPlan:              *userPlan,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
}
