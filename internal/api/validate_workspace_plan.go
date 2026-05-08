package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/logger"
)

var ErrWorkspaceFilesLimitExceeded = errors.New("workspace files limit exceeded")
var ErrWorkspaceFoldersLimitExceeded = errors.New("workspace folders limit exceeded")
var ErrWorkspaceUsersLimitExceeded = errors.New("workspace users limit exceeded")
var ErrWorkspaceStorageLimitExceeded = errors.New("workspace storage limit exceeded")
var ErrWorkspacesPerUserLimitExceeded = errors.New("workspaces per user limit exceeded")

type workspaceQuotaChecks struct {
	files   bool
	storage bool
	folders bool
	members bool
}

// validateWorkspacesPerUserLimit checks whether the owner has hit their plan's
// workspace count cap. Call this before creating a new workspace.
func (s *APIServer) validateWorkspacesPerUserLimit(ctx context.Context, ownerID uuid.UUID) (exceedInfo map[string]any, err error) {
	log := logger.FromContext(ctx)
	exceeded, err := s.repository.Queries().CheckWorkspacesPerUserQuota(ctx, ownerID)
	if err != nil {
		log.Errorw("DB error checking workspace count", "owner", ownerID, "error", err)
		return nil, err
	}
	if exceeded {
		log.Warnw("plan limit exceeded: workspaces per user", "owner", ownerID)
		return map[string]any{
			"msg": "You have reached the maximum number of workspaces for your plan",
		}, ErrWorkspacesPerUserLimitExceeded
	}
	return nil, nil
}

// validateWorkspaceResourceLimits checks the requested per-workspace resource
// caps for a specific operation. pendingBytes is the size of the incoming
// upload (0 for non-upload operations).
func (s *APIServer) validateWorkspaceResourceLimits(ctx context.Context, workspaceID uuid.UUID, pendingBytes int64, checks workspaceQuotaChecks) (exceedInfo map[string]any, err error) {
	log := logger.FromContext(ctx)
	quota, err := s.repository.Queries().GetWorkspaceQuotaDetails(ctx, workspaceID)
	if err != nil {
		log.Errorw("DB error checking workspace resource quota", "workspace_id", workspaceID, "error", err)
		return nil, err
	}

	switch {
	case checks.files && quota.FileCount >= quota.MaxFilesWorkspace:
		log.Warnw("plan limit exceeded: workspace files", "workspace_id", workspaceID)
		return map[string]any{
			"msg": "This workspace has reached the maximum number of files for your plan",
		}, ErrWorkspaceFilesLimitExceeded
	case checks.storage && quota.TotalBytes+pendingBytes > quota.MaxTotalStorageBytesWorkspace:
		log.Warnw("plan limit exceeded: workspace storage", "workspace_id", workspaceID)
		return map[string]any{
			"msg": "This workspace has reached the maximum storage quota for your plan",
		}, ErrWorkspaceStorageLimitExceeded
	case checks.folders && quota.FolderCount >= quota.MaxWorkspaceFolders:
		log.Warnw("plan limit exceeded: workspace folders", "workspace_id", workspaceID)
		return map[string]any{
			"msg": "This workspace has reached the maximum number of folders for your plan",
		}, ErrWorkspaceFoldersLimitExceeded
	case checks.members && quota.MemberCount >= quota.MaxUsersWorkspace:
		log.Warnw("plan limit exceeded: workspace members", "workspace_id", workspaceID)
		return map[string]any{
			"msg": "This workspace has reached the maximum number of members for your plan",
		}, ErrWorkspaceUsersLimitExceeded
	}
	return nil, nil
}

// resolveWorkspaceRole fetches the caller's role in the workspace identified by
// the {workspace_id} URL param. Returns ("", false) on any error.
func (s *APIServer) resolveWorkspaceRole(r *http.Request, callerID uuid.UUID) (workspaceID uuid.UUID, role string, ok bool) {
	rawID := chi.URLParam(r, "workspace_id")
	workspaceID, err := uuid.Parse(rawID)
	if err != nil {
		return uuid.Nil, "", false
	}
	role, err = s.workspaces.GetUserWorkspaceRole(r.Context(), callerID, workspaceID)
	if err != nil {
		return uuid.Nil, "", false
	}
	return workspaceID, role, true
}

// ── Local helpers ─────────────────────────────────────────────────────────────

// wsNormalizePathParam normalises a URL query param to a canonical workspace path.
func wsNormalizePathParam(p string) string {
	if p == "" {
		return "/"
	}
	return p
}

func wsCanWrite(role string) bool {
	return role == "owner" || role == "admin" || role == "editor"
}
