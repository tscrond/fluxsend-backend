package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/apikeydata"
	"github.com/tscrond/fluxsend-backend/internal/logger"
	"github.com/tscrond/fluxsend-backend/internal/service"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

type apiKeyRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Scopes      []string `json:"scopes"`
}

type deleteApiKeyRequest struct {
	APIKeyID string `json:"api_key_id"`
}

func getAPIKeyParameters(r *http.Request) (*apiKeyRequest, error) {
	var keyRequest struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Scopes      []string `json:"scopes"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&keyRequest); err != nil {
		return nil, err
	}

	return &apiKeyRequest{
		Name:        keyRequest.Name,
		Description: keyRequest.Description,
		Scopes:      keyRequest.Scopes,
	}, nil
}

func getDeleteAPIKeyParameters(r *http.Request) (*deleteApiKeyRequest, error) {
	var req deleteApiKeyRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return nil, err
	}
	return &req, nil
}

func (s *APIServer) createWorkspaceAPIKey(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
		return
	}

	_, callerID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	workspaceID, role, ok := s.resolveWorkspaceRole(r, callerID)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}
	if !wsCanWriteAPIKeys(role) {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}

	if exceedInfo, err := s.validateWorkspaceAPIKeyLimit(r.Context(), workspaceID); err != nil {
		if errors.Is(err, ErrWorkspaceAPIKeysLimitExceeded) {
			pkg.WriteJSONResponse(w, http.StatusTooManyRequests, "exceeded_plan_limits", exceedInfo)
		} else {
			log.Errorw("api keys per workspace count check failed", "workspace_id", workspaceID, "error", err)
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		}
		return
	}

	defer r.Body.Close()
	apiKeyData, err := getAPIKeyParameters(r)
	if err != nil {
		log.Errorw("wrong request body parameters", "error", err)
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_json")
		return
	}

	key, err := pkg.GenerateSecureAPIKey()
	if err != nil {
		log.Errorw("failed to generate api key", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	workspaceKeyData, err := apikeydata.NewWorkspaceAPIKeyData(
		apiKeyData.Name,
		apiKeyData.Description,
		key,
		workspaceID,
		callerID,
		apiKeyData.Scopes,
	)
	if err != nil {
		log.Errorw("invalid workspace api key request", "workspace_id", workspaceID, "error", err)
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_request")
		return
	}

	createResult, err := s.apiKeys.CreateWorkspaceAPIKey(r.Context(), workspaceKeyData)
	if err != nil {
		log.Errorw("failed to create api key", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "api_key_created", map[string]any{
		"api_key": createResult,
	})
}

func (s *APIServer) listWorkspaceAPIKeys(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
		return
	}

	_, callerID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	workspaceID, role, ok := s.resolveWorkspaceRole(r, callerID)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}
	if !wsCanWriteAPIKeys(role) {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}

	apiKeys, err := s.apiKeys.ListWorkspaceAPIKeys(r.Context(), workspaceID)
	if err != nil {
		log.Errorw("failed to list workspace api keys", "workspace_id", workspaceID, "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "ok", map[string]any{
		"api_keys": apiKeys,
	})

}

func (s *APIServer) deleteWorkspaceAPIKey(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodDelete {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
		return
	}

	_, callerID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	workspaceID, role, ok := s.resolveWorkspaceRole(r, callerID)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}
	if !wsCanWriteAPIKeys(role) {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "", "forbidden")
		return
	}

	defer r.Body.Close()
	req, err := getDeleteAPIKeyParameters(r)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_json")
		return
	}

	apiKeyID, err := uuid.Parse(req.APIKeyID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_api_key_id")
		return
	}

	if err := s.apiKeys.DeleteWorkspaceAPIKey(r.Context(), workspaceID, apiKeyID, callerID); err != nil {
		if errors.Is(err, service.ErrWorkspaceAPIKeyNotFound) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "", "not_found")
			return
		}
		log.Errorw("failed to delete workspace api key", "workspace_id", workspaceID, "api_key_id", apiKeyID, "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "deleted", nil)
}

func (s *APIServer) createPrivateAPIKey(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
		return
	}

	authorizedUser, ok := parseAuthorizedUserWithPlan(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	callerID, err := uuid.Parse(authorizedUser.InternalID)
	if err != nil {
		log.Errorw("failed to parse authorized user id", "internal_id", authorizedUser.InternalID, "error", err)
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	if exceedInfo, err := s.validatePrivateAPIKeyLimit(
		r.Context(),
		callerID,
		authorizedUser.UserPlan,
	); err != nil {
		if errors.Is(err, ErrPrivateAPIKeysLimitExceeded) {
			pkg.WriteJSONResponse(w, http.StatusTooManyRequests, "exceeded_plan_limits", exceedInfo)
		} else {
			log.Errorw("api keys per user count check failed", "user_id", callerID, "error", err)
			pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		}
		return
	}

	defer r.Body.Close()
	apiKeyData, err := getAPIKeyParameters(r)
	if err != nil {
		log.Errorw("wrong request body parameters", "error", err)
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_json")
		return
	}

	key, err := pkg.GenerateSecureAPIKey()
	if err != nil {
		log.Errorw("failed to generate api key", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	privateKeyData, err := apikeydata.NewPrivateAPIKeyData(
		apiKeyData.Name,
		apiKeyData.Description,
		key,
		callerID,
		apiKeyData.Scopes,
	)
	if err != nil {
		log.Errorw("invalid private api key request", "user_id", callerID, "error", err)
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_request")
		return
	}

	createResult, err := s.apiKeys.CreatePrivateAPIKey(r.Context(), privateKeyData)
	if err != nil {
		log.Errorw("failed to create api key", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "api_key_created", map[string]any{
		"api_key": createResult,
	})
}

func (s *APIServer) listPrivateAPIKeys(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
		return
	}

	_, callerID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	apiKeys, err := s.apiKeys.ListPrivateAPIKeys(r.Context(), callerID)
	if err != nil {
		log.Errorw("failed to list user api keys", "user_id", callerID, "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "ok", map[string]any{
		"api_keys": apiKeys,
	})
}

func (s *APIServer) deletePrivateAPIKey(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodDelete {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "", "method_not_allowed")
		return
	}

	_, callerID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	defer r.Body.Close()
	req, err := getDeleteAPIKeyParameters(r)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_json")
		return
	}

	apiKeyID, err := uuid.Parse(req.APIKeyID)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "invalid_api_key_id")
		return
	}

	if err := s.apiKeys.DeletePrivateAPIKey(r.Context(), callerID, apiKeyID); err != nil {
		if errors.Is(err, service.ErrWorkspaceAPIKeyNotFound) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "", "not_found")
			return
		}
		log.Errorw("failed to delete user api key", "user_id", callerID, "api_key_id", apiKeyID, "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "", "internal_error")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "deleted", nil)
}
