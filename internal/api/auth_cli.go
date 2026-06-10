package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/logger"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"github.com/tscrond/fluxsend-backend/internal/scope"
	"github.com/tscrond/fluxsend-backend/internal/userdata"
	"github.com/tscrond/fluxsend-backend/pkg"
)

type apiKeyBindingType string

const (
	apiKeyBindingPrivate   apiKeyBindingType = "private"
	apiKeyBindingWorkspace apiKeyBindingType = "workspace"
)

type routeDomain string

const (
	routeDomainPrivate   routeDomain = "private"
	routeDomainWorkspace routeDomain = "workspace"
)

type cliKeyBindingContextKey struct{}

var cliKeyBindingCtxKey = cliKeyBindingContextKey{}

type cliKeyBinding struct {
	bindingType apiKeyBindingType
	workspaceID uuid.UUID
}

func (s *CLIServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			log := logger.FromContext(r.Context())
			apiKey := strings.TrimSpace(r.Header.Get("X-API-Key"))
			if apiKey == "" {
				pkg.WriteJSONResponse(w, http.StatusForbidden, "missing_api_key", "Unauthorized")
				log.Errorf("missing API key")
				return
			}

			authorizedCLIUser, err := s.repository.Queries().GetAuthorizedCLIUserInfoByAPIKey(r.Context(), apiKey)
			if err != nil {
				log.Errorw("cannot find valid API key", "error", err)
				pkg.WriteJSONResponse(w, http.StatusForbidden, "invalid_api_key", "Unauthorized")
				return
			}

			bindingType, principalUserID, keyWorkspaceID, ok := parseAPIKeyBinding(authorizedCLIUser)
			if !ok {
				log.Errorw("invalid API key assignment", "api_key_id", authorizedCLIUser.ApiKeyID)
				pkg.WriteJSONResponse(w, http.StatusForbidden, "invalid_api_key_binding", "Unauthorized")
				return
			}

			scopes, err := s.repository.Queries().ListAPIKeyScopes(r.Context(), authorizedCLIUser.ApiKeyID)
			if err != nil {
				log.Errorw("cannot get api key scopes", "error", err)
				pkg.WriteJSONResponse(w, http.StatusInternalServerError, "cannot_get_api_key_scopes", "Internal server error")
				return
			}

			for _, granted := range scopes {
				if !scope.IsKnown(granted) {
					log.Warnw("api key has unknown scope", "scope", granted, "api_key_id", authorizedCLIUser.ApiKeyID)
					pkg.WriteJSONResponse(w, http.StatusForbidden, "invalid_scope", "Unauthorized")
					return
				}
			}

			userPlan, err := s.repository.Queries().GetUserPlan(r.Context(), principalUserID)
			if err != nil {
				log.Errorw("cannot get user plan", "error", err)
				pkg.WriteJSONResponse(w, http.StatusInternalServerError, "cannot_get_user_plan", "Internal server error")
				return
			}

			mappedUserPlan := userdata.UserPlan{
				PlanID:                        userPlan.ID.String(),
				PlanName:                      userPlan.Name,
				MaxFileSizeBytes:              userPlan.MaxFileSizeBytes,
				MaxTotalStorageBytes:          userPlan.MaxTotalStorageBytes,
				MaxFiles:                      userPlan.MaxFiles,
				MaxFilesSentPerDay:            userPlan.MaxFilesSentPerDay,
				MaxSharesPerDay:               userPlan.MaxSharesPerDay,
				MaxFilesWorkspace:             userPlan.MaxFilesWorkspace,
				MaxUserWorkspaces:             userPlan.MaxUserWorkspaces,
				MaxWorkspaceFolders:           userPlan.MaxWorkspaceFolders,
				MaxUsersPerWorkspace:          userPlan.MaxUsersWorkspace,
				MaxTotalStorageBytesWorkspace: userPlan.MaxTotalStorageBytesWorkspace,
				MaxPrivateAPIKeys:             userPlan.MaxPrivateApiKeys,
				MaxWorkspaceAPIKeys:           userPlan.MaxWorkspaceApiKeys,
			}

			switch bindingType {
			case apiKeyBindingPrivate:
				if mappedUserPlan.MaxPrivateAPIKeys == 0 {
					log.Errorw("user not allowed to use private api keys", "user_id", principalUserID)
					pkg.WriteJSONResponse(w, http.StatusForbidden, "api_usage_not_allowed", "Your plan does not allow API key usage")
					return
				}
			case apiKeyBindingWorkspace:
				if mappedUserPlan.MaxWorkspaceAPIKeys == 0 {
					log.Errorw("user not allowed to use workspace api keys", "user_id", principalUserID)
					pkg.WriteJSONResponse(w, http.StatusForbidden, "api_usage_not_allowed", "Your plan does not allow API key usage")
					return
				}
			}

			// Adapt AuthorizedCLIUserInfo to AuthorizedUserInfo to unify handler logic
			authorizedUser := &userdata.AuthorizedUserInfo{
				InternalID: principalUserID.String(),
				Email:      authorizedCLIUser.Email,
				Name:       authorizedCLIUser.Name.String,
				Provider:   "api", // Static provider for API-based auth
			}

			ctx := context.WithValue(r.Context(), userdata.APIKeyScopesContextKey, scopes)
			ctx = context.WithValue(ctx, cliKeyBindingCtxKey, cliKeyBinding{
				bindingType: bindingType,
				workspaceID: keyWorkspaceID,
			})
			ctx = context.WithValue(ctx, userdata.AuthorizedUserWithPlanContextKey, &userdata.AuthorizedUserWithPlan{
				AuthorizedUserInfo: *authorizedUser,
				UserPlan:           mappedUserPlan,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
}

func parseAPIKeyBinding(authorizedCLIUser sqlc.GetAuthorizedCLIUserInfoByAPIKeyRow) (apiKeyBindingType, uuid.UUID, uuid.UUID, bool) {
	privateBound := authorizedCLIUser.PrivateUserID.Valid
	workspaceBound := authorizedCLIUser.WorkspaceID.Valid

	if privateBound == workspaceBound {
		return "", uuid.Nil, uuid.Nil, false
	}

	if privateBound {
		return apiKeyBindingPrivate, authorizedCLIUser.PrivateUserID.UUID, uuid.Nil, true
	}

	return apiKeyBindingWorkspace, authorizedCLIUser.InternalID, authorizedCLIUser.WorkspaceID.UUID, true
}

func bindingAllowsDomain(binding apiKeyBindingType, domain routeDomain) bool {
	if binding == apiKeyBindingPrivate {
		return domain == routeDomainPrivate
	}
	return domain == routeDomainWorkspace
}

func extractWorkspaceIDFromRequest(r *http.Request) (uuid.UUID, bool) {
	if raw := strings.TrimSpace(chi.URLParam(r, "workspace_id")); raw != "" {
		if parsed, err := uuid.Parse(raw); err == nil {
			return parsed, true
		}
	}

	if raw := strings.TrimSpace(r.URL.Query().Get("workspace_id")); raw != "" {
		if parsed, err := uuid.Parse(raw); err == nil {
			return parsed, true
		}
	}

	if r.Body == nil {
		return uuid.Nil, false
	}

	bodyBytes, err := readAndRestoreBody(r)
	if err != nil || len(bodyBytes) == 0 {
		return uuid.Nil, false
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		return uuid.Nil, false
	}

	rawWorkspaceID, ok := payload["workspace_id"]
	if !ok {
		return uuid.Nil, false
	}

	var workspaceID string
	if err := json.Unmarshal(rawWorkspaceID, &workspaceID); err != nil {
		return uuid.Nil, false
	}

	parsed, err := uuid.Parse(strings.TrimSpace(workspaceID))
	if err != nil {
		return uuid.Nil, false
	}

	return parsed, true
}

func readAndRestoreBody(r *http.Request) ([]byte, error) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	return bodyBytes, nil
}

func (s *CLIServer) requireKeyBinding(domain routeDomain, requiresWorkspaceID bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log := logger.FromContext(r.Context())
			binding, ok := r.Context().Value(cliKeyBindingCtxKey).(cliKeyBinding)
			if !ok {
				log.Errorw("could not retrieve api key binding from context")
				pkg.WriteJSONResponse(w, http.StatusInternalServerError, "cannot_retrieve_key_binding", "Internal server error")
				return
			}

			if !bindingAllowsDomain(binding.bindingType, domain) {
				log.Warnw("api key binding does not allow route domain", "binding_type", binding.bindingType, "domain", domain, "path", r.URL.Path)
				pkg.WriteJSONResponse(w, http.StatusForbidden, "insufficient_scope", "You don't have permission to perform this action")
				return
			}

			if binding.bindingType == apiKeyBindingWorkspace && requiresWorkspaceID {
				requestedWorkspaceID, found := extractWorkspaceIDFromRequest(r)
				if !found {
					log.Warnw("workspace-scoped api key requires explicit workspace id", "path", r.URL.Path, "method", r.Method)
					pkg.WriteJSONResponse(w, http.StatusForbidden, "workspace_scope_violation", "You don't have permission to perform this action")
					return
				}
				if requestedWorkspaceID != binding.workspaceID {
					log.Warnw("workspace-scoped api key attempted access outside assigned workspace", "assigned_workspace_id", binding.workspaceID, "requested_workspace_id", requestedWorkspaceID)
					pkg.WriteJSONResponse(w, http.StatusForbidden, "workspace_scope_violation", "You don't have permission to perform this action")
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (s *CLIServer) requireScope(requiredScope scope.Scope) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log := logger.FromContext(r.Context())
			scopes, ok := r.Context().Value(userdata.APIKeyScopesContextKey).([]string)
			if !ok {
				log.Errorw("could not retrieve scopes from context")
				pkg.WriteJSONResponse(w, http.StatusInternalServerError, "cannot_retrieve_scopes", "Internal server error")
				return
			}

			for _, s := range scopes {
				if s == requiredScope.String() {
					next.ServeHTTP(w, r)
					return
				}
			}

			log.Warnw("user does not have required scope", "required_scope", requiredScope)
			pkg.WriteJSONResponse(w, http.StatusForbidden, "insufficient_scope", "You don't have permission to perform this action")
		})
	}
}
