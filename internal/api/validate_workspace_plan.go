package api

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

var ErrWorkspaceFilesLimitExceeded = errors.New("workspace files limit exceeded")
var ErrWorkspaceFoldersLimitExceeded = errors.New("workspace folders limit exceeded")
var ErrWorkspaceUsersLimitExceeded = errors.New("workspace users limit exceeded")
var ErrWorkspaceStorageLimitExceeded = errors.New("workspace storage limit exceeded")
var ErrWorkspacesPerUserLimitExceeded = errors.New("workspaces per user limit exceeded")

// validateWorkspacesPerUserLimit checks whether the owner has hit their plan's
// workspace count cap. Call this before creating a new workspace.
func (s *APIServer) validateWorkspacesPerUserLimit(ctx context.Context, ownerID uuid.UUID) (exceedInfo map[string]any, err error) {
	exceeded, err := s.repository.Queries().CheckWorkspacesPerUserQuota(ctx, ownerID)
	if err != nil {
		log.Printf("[plan-limit] DB error checking workspace count for owner=%s: %v", ownerID, err)
		return nil, err
	}
	if exceeded {
		log.Printf("[plan-limit] owner=%s: workspaces per user limit exceeded", ownerID)
		return map[string]any{
			"msg": "You have reached the maximum number of workspaces for your plan",
		}, ErrWorkspacesPerUserLimitExceeded
	}
	return nil, nil
}

// validateWorkspaceResourceLimits checks whether a specific workspace has hit
// its plan's per-workspace resource caps (files, storage, folders, members).
// pendingBytes is the size of the incoming upload (0 for non-upload operations).
// Call this before adding a file, folder, or member to a workspace.
func (s *APIServer) validateWorkspaceResourceLimits(ctx context.Context, workspaceID uuid.UUID, pendingBytes int64) (exceedInfo map[string]any, err error) {
	quota, err := s.repository.Queries().GetWorkspaceQuotaDetails(ctx, workspaceID)
	if err != nil {
		log.Printf("[plan-limit] DB error checking resource quota for workspace=%s: %v", workspaceID, err)
		return nil, err
	}

	switch {
	case quota.FileCount >= quota.MaxFilesWorkspace:
		log.Printf("[plan-limit] workspace=%s: files limit exceeded", workspaceID)
		return map[string]any{
			"msg": "This workspace has reached the maximum number of files for your plan",
		}, ErrWorkspaceFilesLimitExceeded
	case quota.TotalBytes+pendingBytes > quota.MaxTotalStorageBytesWorkspace:
		log.Printf("[plan-limit] workspace=%s: storage limit exceeded", workspaceID)
		return map[string]any{
			"msg": "This workspace has reached the maximum storage quota for your plan",
		}, ErrWorkspaceStorageLimitExceeded
	case quota.FolderCount >= quota.MaxWorkspaceFolders:
		log.Printf("[plan-limit] workspace=%s: folders limit exceeded", workspaceID)
		return map[string]any{
			"msg": "This workspace has reached the maximum number of folders for your plan",
		}, ErrWorkspaceFoldersLimitExceeded
	case quota.MemberCount >= quota.MaxUsersWorkspace:
		log.Printf("[plan-limit] workspace=%s: members limit exceeded", workspaceID)
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
