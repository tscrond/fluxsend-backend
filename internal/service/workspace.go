package service

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/repo"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
)

type WorkspaceResult struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	OwnerID     uuid.UUID `json:"owner_id"`
	CreatedAt   string    `json:"created_at"`
	Role        string    `json:"role"`
}

type WorkspaceMemberResult struct {
	UserID   uuid.UUID `json:"user_id"`
	Role     string    `json:"role"`
	JoinedAt string    `json:"joined_at"`
	Email    string    `json:"email"`
}

type WorkspaceInviteResult struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	ExpiresAt   string    `json:"expires_at"`
}

type UserInviteResult struct {
	ID            uuid.UUID `json:"id"`
	WorkspaceID   uuid.UUID `json:"workspace_id"`
	WorkspaceName string    `json:"workspace_name"`
	WorkspaceSlug string    `json:"workspace_slug"`
	Email         string    `json:"email"`
	Token         string    `json:"token"`
	Role          string    `json:"role"`
	ExpiresAt     string    `json:"expires_at"`
}

type WorkspaceService interface {
	GetUserWorkspaces(ctx context.Context, userID uuid.UUID) ([]WorkspaceResult, error)
	CreateWorkspace(ctx context.Context, userID uuid.UUID, name, slug string) error
	DeleteWorkspace(ctx context.Context, workspaceID uuid.UUID) error
	RenameWorkspace(ctx context.Context, workspaceID uuid.UUID, newName, newSlug string) (*WorkspaceResult, error)
	GetWorkspaceMembers(ctx context.Context, workspaceID uuid.UUID) ([]WorkspaceMemberResult, error)
	GetWorkspaceInvites(ctx context.Context, workspaceID uuid.UUID) ([]WorkspaceInviteResult, error)
	CreateWorkspaceInvite(ctx context.Context, workspaceID uuid.UUID, email, token, role string) (*WorkspaceInviteResult, error)
	DeleteWorkspaceInvite(ctx context.Context, inviteID uuid.UUID) error
	GetUserInvites(ctx context.Context, email string) ([]UserInviteResult, error)
	AcceptWorkspaceInvite(ctx context.Context, token string, userID uuid.UUID, userEmail string) error
	RejectWorkspaceInvite(ctx context.Context, token string, userEmail string) error
	GetUserWorkspaceRole(ctx context.Context, userId uuid.UUID, workspaceId uuid.UUID) (string, error)
	RemoveWorkspaceMember(ctx context.Context, workspaceID uuid.UUID, userID uuid.UUID) error
	UpdateMemberRole(ctx context.Context, workspaceID uuid.UUID, targetUserID uuid.UUID, newRole string) error
}

type workspaceService struct {
	queries *sqlc.Queries
	repo    repo.Repository
}

func NewWorkspaceService(queries *sqlc.Queries, r repo.Repository) WorkspaceService {
	return &workspaceService{
		queries: queries,
		repo:    r,
	}
}

func (w *workspaceService) GetUserWorkspaces(ctx context.Context, userID uuid.UUID) ([]WorkspaceResult, error) {
	workspaces, err := w.queries.GetUserWorkspaces(ctx, userID)
	if err != nil {
		log.Printf("error getting user workspaces: %v", err)
		return nil, err
	}

	result := make([]WorkspaceResult, 0, len(workspaces))
	for _, ws := range workspaces {
		result = append(result, WorkspaceResult{
			WorkspaceID: ws.ID,
			Name:        ws.Name,
			Slug:        ws.Slug,
			OwnerID:     ws.OwnerID,
			CreatedAt:   ws.CreatedAt.Format(time.RFC3339),
			Role:        ws.Role,
		})
	}
	return result, nil
}

func (w *workspaceService) GetUserWorkspaceRole(ctx context.Context, userId uuid.UUID, workspaceId uuid.UUID) (string, error) {
	role, err := w.queries.GetUserWorkspaceRole(ctx, sqlc.GetUserWorkspaceRoleParams{
		WorkspaceID: workspaceId,
		UserID:      userId,
	})
	if err != nil {
		log.Printf("error getting user workspace role: %v", err)
		return "", err
	}
	return role, nil
}

func (w *workspaceService) CreateWorkspace(ctx context.Context, userID uuid.UUID, name, slug string) error {
	tx, err := w.repo.BeginTx(ctx, nil)
	if err != nil {
		log.Printf("error beginning transaction for CreateWorkspace: %v", err)
		return err
	}
	txq := w.queries.WithTx(tx)

	workspace, err := txq.CreateWorkspace(ctx, sqlc.CreateWorkspaceParams{
		Slug:    slug,
		Name:    name,
		OwnerID: userID,
	})
	if err != nil {
		_ = tx.Rollback()
		log.Printf("error creating workspace: %v", err)
		return err
	}

	_, err = txq.CreateWorkspaceMember(ctx, sqlc.CreateWorkspaceMemberParams{
		WorkspaceID: workspace.ID,
		UserID:      userID,
		Role:        "owner",
	})
	if err != nil {
		_ = tx.Rollback()
		log.Printf("error adding owner to workspace members: %v", err)
		return err
	}

	return tx.Commit()
}

func (w *workspaceService) DeleteWorkspace(ctx context.Context, workspaceID uuid.UUID) error {
	workspaceDeleted, err := w.queries.DeleteWorkspace(ctx, workspaceID)
	if err != nil {
		log.Printf("error deleting workspace: %v", err)
		return err
	}
	log.Printf("deleted workspace: %v (%s)", workspaceID, workspaceDeleted.Slug)
	return nil
}

func (w *workspaceService) RenameWorkspace(ctx context.Context, workspaceID uuid.UUID, newName, newSlug string) (*WorkspaceResult, error) {
	if newName == "" {
		return nil, errors.New("name_required")
	}

	var renameErr error
	var updated sqlc.Workspace

	if utf8.RuneCountInString(newName) > 64 {
		return nil, errors.New("name_too_long")
	}

	if newSlug != "" && utf8.RuneCountInString(newSlug) > 64 {
		return nil, errors.New("slug_too_long")
	}

	if newSlug != "" {
		updated, renameErr = w.queries.RenameWorkspaceWithSlug(ctx, sqlc.RenameWorkspaceWithSlugParams{
			ID:   workspaceID,
			Name: newName,
			Slug: newSlug,
		})
	} else {
		updated, renameErr = w.queries.RenameWorkspace(ctx, sqlc.RenameWorkspaceParams{
			ID:   workspaceID,
			Name: newName,
		})
	}

	if renameErr != nil {
		log.Printf("error renaming workspace %v: %v", workspaceID, renameErr)
		return nil, renameErr
	}
	return &WorkspaceResult{
		WorkspaceID: updated.ID,
		Name:        updated.Name,
		Slug:        updated.Slug,
		OwnerID:     updated.OwnerID,
		CreatedAt:   updated.CreatedAt.Format(time.RFC3339),
	}, nil
}

func (w *workspaceService) GetWorkspaceMembers(ctx context.Context, workspaceID uuid.UUID) ([]WorkspaceMemberResult, error) {
	members, err := w.queries.GetWorkspaceMembers(ctx, workspaceID)
	if err != nil {
		log.Printf("error getting workspace members: %v\n", err)
		return nil, err
	}

	membersResult := make([]WorkspaceMemberResult, 0, len(members))
	for _, member := range members {
		membersResult = append(membersResult, WorkspaceMemberResult{
			UserID:   member.ID,
			Email:    member.UserEmail,
			Role:     member.Role,
			JoinedAt: member.JoinedAt.Format(time.RFC3339),
		})
	}
	return membersResult, nil
}

func (w *workspaceService) GetWorkspaceInvites(ctx context.Context, workspaceID uuid.UUID) ([]WorkspaceInviteResult, error) {
	invites, err := w.queries.GetWorkspaceInvites(ctx, workspaceID)
	if err != nil {
		log.Printf("error getting workspace invites: %v\n", err)
		return nil, err
	}

	invitesResult := make([]WorkspaceInviteResult, 0, len(invites))
	for _, inv := range invites {
		invitesResult = append(invitesResult, WorkspaceInviteResult{
			ID:          inv.ID,
			WorkspaceID: inv.WorkspaceID,
			Role:        inv.Role,
			Email:       inv.Email,
			ExpiresAt:   inv.ExpiresAt.Format(time.RFC3339),
		})
	}
	return invitesResult, nil
}

func (w *workspaceService) CreateWorkspaceInvite(ctx context.Context, workspaceID uuid.UUID, email, token, role string) (*WorkspaceInviteResult, error) {
	// If the user already has an account, check they're not already a member.
	user, err := w.queries.GetUserByEmail(ctx, email)
	if err == nil {
		_, memberErr := w.queries.GetWorkspaceMember(ctx, sqlc.GetWorkspaceMemberParams{
			WorkspaceID: workspaceID,
			UserID:      user.ID,
		})
		if memberErr == nil {
			return nil, errors.New("already_a_member")
		} else if !errors.Is(memberErr, sql.ErrNoRows) {
			log.Printf("error looking up workspace member: %v", memberErr)
			return nil, memberErr
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		log.Printf("error looking up user by email: %v", err)
		return nil, err
	}

	// Always create an invite row — the user must accept explicitly.
	inv, err := w.queries.CreateWorkspaceInvite(ctx, sqlc.CreateWorkspaceInviteParams{
		WorkspaceID: workspaceID,
		Email:       email,
		Token:       token,
		Role:        role,
	})
	if err != nil {
		log.Printf("error creating workspace invite: %v", err)
		return nil, err
	}
	return &WorkspaceInviteResult{
		ID:          inv.ID,
		WorkspaceID: inv.WorkspaceID,
		Email:       inv.Email,
		Role:        inv.Role,
		ExpiresAt:   inv.ExpiresAt.Format(time.RFC3339),
	}, nil
}

func (w *workspaceService) RemoveWorkspaceMember(ctx context.Context, workspaceID uuid.UUID, userID uuid.UUID) error {
	if err := w.queries.DeleteWorkspaceMember(ctx, sqlc.DeleteWorkspaceMemberParams{
		WorkspaceID: workspaceID,
		UserID:      userID,
	}); err != nil {
		log.Printf("error removing workspace member %v from %v: %v", userID, workspaceID, err)
		return err
	}
	return nil
}

func (w *workspaceService) UpdateMemberRole(ctx context.Context, workspaceID uuid.UUID, targetUserID uuid.UUID, newRole string) error {
	if err := w.queries.UpdateWorkspaceMemberRole(ctx, sqlc.UpdateWorkspaceMemberRoleParams{
		WorkspaceID: workspaceID,
		UserID:      targetUserID,
		Role:        newRole,
	}); err != nil {
		log.Printf("error updating role for member %v in workspace %v: %v", targetUserID, workspaceID, err)
		return err
	}
	return nil
}

func (w *workspaceService) DeleteWorkspaceInvite(ctx context.Context, inviteID uuid.UUID) error {
	if err := w.queries.DeleteWorkspaceInvite(ctx, inviteID); err != nil {
		log.Printf("error deleting workspace invite %v: %v", inviteID, err)
		return err
	}
	return nil
}

func (w *workspaceService) GetUserInvites(ctx context.Context, email string) ([]UserInviteResult, error) {
	rows, err := w.queries.GetUserInvitesByEmail(ctx, email)
	if err != nil {
		log.Printf("error getting user invites for %s: %v", email, err)
		return nil, err
	}
	result := make([]UserInviteResult, 0, len(rows))
	for _, r := range rows {
		result = append(result, UserInviteResult{
			ID:            r.ID,
			WorkspaceID:   r.WorkspaceID,
			WorkspaceName: r.WorkspaceName,
			WorkspaceSlug: r.WorkspaceSlug,
			Email:         r.Email,
			Token:         r.Token,
			Role:          r.Role,
			ExpiresAt:     r.ExpiresAt.Format(time.RFC3339),
		})
	}
	return result, nil
}

func (w *workspaceService) AcceptWorkspaceInvite(ctx context.Context, token string, userID uuid.UUID, userEmail string) error {
	invite, err := w.queries.GetWorkspaceInviteByToken(ctx, token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("invite_not_found")
		}
		return err
	}
	if invite.ExpiresAt.Before(time.Now()) {
		return errors.New("invite_expired")
	}
	if !strings.EqualFold(invite.Email, userEmail) {
		return errors.New("invite_not_for_you")
	}
	// Already a member?
	_, memberErr := w.queries.GetWorkspaceMember(ctx, sqlc.GetWorkspaceMemberParams{
		WorkspaceID: invite.WorkspaceID,
		UserID:      userID,
	})
	if memberErr == nil {
		// Already a member — still delete the stale invite.
		_ = w.queries.DeleteWorkspaceInviteByToken(ctx, token)
		return errors.New("already_a_member")
	}
	if !errors.Is(memberErr, sql.ErrNoRows) {
		return memberErr
	}
	if _, err := w.queries.CreateWorkspaceMember(ctx, sqlc.CreateWorkspaceMemberParams{
		WorkspaceID: invite.WorkspaceID,
		UserID:      userID,
		Role:        invite.Role,
	}); err != nil {
		log.Printf("error accepting workspace invite: %v", err)
		return err
	}
	return w.queries.DeleteWorkspaceInviteByToken(ctx, token)
}

func (w *workspaceService) RejectWorkspaceInvite(ctx context.Context, token string, userEmail string) error {
	invite, err := w.queries.GetWorkspaceInviteByToken(ctx, token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("invite_not_found")
		}
		return err
	}
	if !strings.EqualFold(invite.Email, userEmail) {
		return errors.New("invite_not_for_you")
	}
	return w.queries.DeleteWorkspaceInviteByToken(ctx, token)
}
