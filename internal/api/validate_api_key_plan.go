package api

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/logger"
	"github.com/tscrond/fluxsend-backend/internal/userdata"
)

var ErrPrivateAPIKeysLimitExceeded = errors.New("private api keys limit exceeded")
var ErrWorkspaceAPIKeysLimitExceeded = errors.New("workspace api keys limit exceeded")

func (s *APIServer) validatePrivateAPIKeyLimit(ctx context.Context, userID uuid.UUID, userPlan userdata.UserPlan) (map[string]any, error) {
	log := logger.FromContext(ctx)
	exceeded, err := s.repository.Queries().CheckPrivateAPIKeyQuota(ctx, userID)
	if err != nil {
		log.Errorw("DB error checking private api key quota", "user_id", userID, "error", err)
		return nil, err
	}
	if exceeded {
		log.Warnw("plan limit exceeded: private api keys", "user_id", userID, "limit", userPlan.MaxPrivateAPIKeys)
		return map[string]any{
			"msg":                  "You have reached the maximum number of API keys for private files on your plan",
			"max_private_api_keys": userPlan.MaxPrivateAPIKeys,
		}, ErrPrivateAPIKeysLimitExceeded
	}
	return nil, nil
}

func (s *APIServer) validateWorkspaceAPIKeyLimit(ctx context.Context, workspaceID uuid.UUID) (map[string]any, error) {
	log := logger.FromContext(ctx)
	exceeded, err := s.repository.Queries().CheckWorkspaceAPIKeyQuota(ctx, workspaceID)
	if err != nil {
		log.Errorw("DB error checking workspace api key quota", "workspace_id", workspaceID, "error", err)
		return nil, err
	}
	if exceeded {
		quota, err := s.repository.Queries().GetWorkspaceQuotaDetails(ctx, workspaceID)
		if err != nil {
			log.Errorw("DB error loading workspace quota after api key limit breach", "workspace_id", workspaceID, "error", err)
			return nil, err
		}
		log.Warnw("plan limit exceeded: workspace api keys", "workspace_id", workspaceID, "limit", quota.MaxWorkspaceApiKeys)
		return map[string]any{
			"msg":                    "This workspace has reached the maximum number of API keys for your plan",
			"max_workspace_api_keys": quota.MaxWorkspaceApiKeys,
		}, ErrWorkspaceAPIKeysLimitExceeded
	}
	return nil, nil
}
