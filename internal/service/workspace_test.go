package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/tscrond/fluxsend-backend/internal/mocks"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
)

func newWsSvc(q *mocks.MockQuerier, repo *mocks.MockRepository) WorkspaceService {
	return NewWorkspaceService(q, repo)
}

// --- GetUserWorkspaces ---------------------------------------------------

func TestWorkspaceService_GetUserWorkspaces_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	userID := uuid.New()
	wsID := uuid.New()

	q.EXPECT().GetUserWorkspaces(gomock.Any(), userID).Return([]sqlc.GetUserWorkspacesRow{{
		ID:        wsID,
		Slug:      "my-ws",
		Name:      "My Workspace",
		OwnerID:   userID,
		CreatedAt: time.Now(),
		Role:      "owner",
	}}, nil)

	result, err := svc.GetUserWorkspaces(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, wsID, result[0].WorkspaceID)
	assert.Equal(t, "owner", result[0].Role)
}

func TestWorkspaceService_GetUserWorkspaces_DBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	dbErr := errors.New("db unavailable")
	q.EXPECT().GetUserWorkspaces(gomock.Any(), gomock.Any()).Return(nil, dbErr)

	_, err := svc.GetUserWorkspaces(context.Background(), uuid.New())
	assert.ErrorIs(t, err, dbErr)
}

// --- DeleteWorkspace -----------------------------------------------------

func TestWorkspaceService_DeleteWorkspace_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	wsID := uuid.New()
	q.EXPECT().DeleteWorkspace(gomock.Any(), wsID).
		Return(sqlc.Workspace{ID: wsID, Slug: "ws-slug"}, nil)

	err := svc.DeleteWorkspace(context.Background(), wsID)
	assert.NoError(t, err)
}

func TestWorkspaceService_DeleteWorkspace_DBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	dbErr := errors.New("workspace not found")
	q.EXPECT().DeleteWorkspace(gomock.Any(), gomock.Any()).Return(sqlc.Workspace{}, dbErr)

	err := svc.DeleteWorkspace(context.Background(), uuid.New())
	assert.ErrorIs(t, err, dbErr)
}

// --- RenameWorkspace -----------------------------------------------------

func TestWorkspaceService_RenameWorkspace_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	wsID := uuid.New()
	updated := sqlc.Workspace{
		ID:        wsID,
		Name:      "New Name",
		Slug:      "new-slug",
		CreatedAt: time.Now(),
	}
	q.EXPECT().RenameWorkspaceWithSlug(gomock.Any(), sqlc.RenameWorkspaceWithSlugParams{
		ID:   wsID,
		Name: "New Name",
		Slug: "new-slug",
	}).Return(updated, nil)

	result, err := svc.RenameWorkspace(context.Background(), wsID, "New Name", "new-slug")
	require.NoError(t, err)
	assert.Equal(t, "New Name", result.Name)
	assert.Equal(t, "new-slug", result.Slug)
}

func TestWorkspaceService_RenameWorkspace_NameTooLong(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	// 65 chars > 64 limit
	longName := string(make([]byte, 65))

	_, err := svc.RenameWorkspace(context.Background(), uuid.New(), longName, "")
	assert.Error(t, err)
}

func TestWorkspaceService_RenameWorkspace_SlugTooLong(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	longSlug := string(make([]byte, 65))

	_, err := svc.RenameWorkspace(context.Background(), uuid.New(), "Valid Name", longSlug)
	assert.Error(t, err)
}

func TestWorkspaceService_RenameWorkspace_EmptyName(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	_, err := svc.RenameWorkspace(context.Background(), uuid.New(), "", "slug")
	assert.Error(t, err)
}

// --- GetWorkspaceMembers -------------------------------------------------

func TestWorkspaceService_GetWorkspaceMembers_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	wsID := uuid.New()
	memberID := uuid.New()

	q.EXPECT().GetWorkspaceMembers(gomock.Any(), wsID).Return([]sqlc.GetWorkspaceMembersRow{{
		ID:        memberID,
		UserEmail: "alice@example.com",
		Role:      "editor",
		JoinedAt:  time.Now(),
	}}, nil)

	members, err := svc.GetWorkspaceMembers(context.Background(), wsID)
	require.NoError(t, err)
	require.Len(t, members, 1)
	assert.Equal(t, "alice@example.com", members[0].Email)
	assert.Equal(t, "editor", members[0].Role)
}

// --- GetWorkspaceInvites -------------------------------------------------

func TestWorkspaceService_GetWorkspaceInvites_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	wsID := uuid.New()
	invID := uuid.New()
	expiresAt := time.Now().Add(time.Hour)

	q.EXPECT().GetWorkspaceInvites(gomock.Any(), wsID).Return([]sqlc.WorkspaceInvite{{
		ID:          invID,
		WorkspaceID: wsID,
		Email:       "newuser@example.com",
		Role:        "viewer",
		ExpiresAt:   expiresAt,
	}}, nil)

	invites, err := svc.GetWorkspaceInvites(context.Background(), wsID)
	require.NoError(t, err)
	require.Len(t, invites, 1)
	assert.Equal(t, "newuser@example.com", invites[0].Email)
}

// --- CreateWorkspaceInvite -----------------------------------------------

func TestWorkspaceService_CreateWorkspaceInvite_NewUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	wsID := uuid.New()
	invID := uuid.New()
	const email = "newuser@example.com"
	const tok = "invite-tok-123"
	expiresAt := time.Now().Add(24 * time.Hour)

	// User doesn't exist yet → sql.ErrNoRows
	q.EXPECT().GetUserByEmail(gomock.Any(), email).Return(sqlc.User{}, sql.ErrNoRows)

	q.EXPECT().CreateWorkspaceInvite(gomock.Any(), sqlc.CreateWorkspaceInviteParams{
		WorkspaceID: wsID, Email: email, Token: tok, Role: "viewer",
	}).Return(sqlc.WorkspaceInvite{
		ID: invID, WorkspaceID: wsID, Email: email, Role: "viewer", ExpiresAt: expiresAt,
	}, nil)

	result, err := svc.CreateWorkspaceInvite(context.Background(), wsID, email, tok, "viewer")
	require.NoError(t, err)
	assert.Equal(t, email, result.Email)
	assert.Equal(t, "viewer", result.Role)
}

func TestWorkspaceService_CreateWorkspaceInvite_AlreadyMember(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	wsID := uuid.New()
	memberID := uuid.New()
	const email = "existing@example.com"

	q.EXPECT().GetUserByEmail(gomock.Any(), email).Return(sqlc.User{ID: memberID}, nil)
	q.EXPECT().GetWorkspaceMember(gomock.Any(), sqlc.GetWorkspaceMemberParams{
		WorkspaceID: wsID, UserID: memberID,
	}).Return(sqlc.WorkspaceMember{}, nil) // already member

	_, err := svc.CreateWorkspaceInvite(context.Background(), wsID, email, "tok", "viewer")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already_a_member")
}

// --- RemoveWorkspaceMember -----------------------------------------------

func TestWorkspaceService_RemoveWorkspaceMember_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	wsID := uuid.New()
	userID := uuid.New()

	q.EXPECT().DeleteWorkspaceMember(gomock.Any(), sqlc.DeleteWorkspaceMemberParams{
		WorkspaceID: wsID, UserID: userID,
	}).Return(nil)

	err := svc.RemoveWorkspaceMember(context.Background(), wsID, userID)
	assert.NoError(t, err)
}

// --- AcceptWorkspaceInvite -----------------------------------------------

func TestWorkspaceService_AcceptWorkspaceInvite_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	wsID := uuid.New()
	invID := uuid.New()
	userID := uuid.New()
	const tok = "invite-tok"
	const email = "user@example.com"

	q.EXPECT().GetWorkspaceInviteByToken(gomock.Any(), tok).Return(sqlc.WorkspaceInvite{
		ID:          invID,
		WorkspaceID: wsID,
		Email:       email,
		Role:        "editor",
		ExpiresAt:   time.Now().Add(time.Hour),
	}, nil)
	// Not yet a member
	q.EXPECT().GetWorkspaceMember(gomock.Any(), sqlc.GetWorkspaceMemberParams{
		WorkspaceID: wsID, UserID: userID,
	}).Return(sqlc.WorkspaceMember{}, sql.ErrNoRows)
	q.EXPECT().CreateWorkspaceMember(gomock.Any(), sqlc.CreateWorkspaceMemberParams{
		WorkspaceID: wsID, UserID: userID, Role: "editor",
	}).Return(sqlc.WorkspaceMember{}, nil)
	q.EXPECT().DeleteWorkspaceInviteByToken(gomock.Any(), tok).Return(nil)

	err := svc.AcceptWorkspaceInvite(context.Background(), tok, userID, email)
	assert.NoError(t, err)
}

func TestWorkspaceService_AcceptWorkspaceInvite_Expired(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	tok := "expired-tok"
	q.EXPECT().GetWorkspaceInviteByToken(gomock.Any(), tok).Return(sqlc.WorkspaceInvite{
		Email:     "user@example.com",
		ExpiresAt: time.Now().Add(-time.Hour),
	}, nil)

	err := svc.AcceptWorkspaceInvite(context.Background(), tok, uuid.New(), "user@example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invite_expired")
}

func TestWorkspaceService_AcceptWorkspaceInvite_WrongEmail(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	tok := "some-tok"
	q.EXPECT().GetWorkspaceInviteByToken(gomock.Any(), tok).Return(sqlc.WorkspaceInvite{
		Email:     "other@example.com",
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil)

	err := svc.AcceptWorkspaceInvite(context.Background(), tok, uuid.New(), "wrong@example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invite_not_for_you")
}

func TestWorkspaceService_AcceptWorkspaceInvite_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	tok := "ghost-tok"
	q.EXPECT().GetWorkspaceInviteByToken(gomock.Any(), tok).Return(sqlc.WorkspaceInvite{}, sql.ErrNoRows)

	err := svc.AcceptWorkspaceInvite(context.Background(), tok, uuid.New(), "user@example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invite_not_found")
}

// --- RejectWorkspaceInvite -----------------------------------------------

func TestWorkspaceService_RejectWorkspaceInvite_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	tok := "rej-tok"
	const email = "user@example.com"

	q.EXPECT().GetWorkspaceInviteByToken(gomock.Any(), tok).Return(sqlc.WorkspaceInvite{
		Email:     email,
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil)
	q.EXPECT().DeleteWorkspaceInviteByToken(gomock.Any(), tok).Return(nil)

	err := svc.RejectWorkspaceInvite(context.Background(), tok, email)
	assert.NoError(t, err)
}

func TestWorkspaceService_RejectWorkspaceInvite_WrongEmail(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	tok := "rej-tok"
	q.EXPECT().GetWorkspaceInviteByToken(gomock.Any(), tok).Return(sqlc.WorkspaceInvite{
		Email:     "owner@example.com",
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil)

	err := svc.RejectWorkspaceInvite(context.Background(), tok, "attacker@example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invite_not_for_you")
}

// --- GetUserInvites -------------------------------------------------------

func TestWorkspaceService_GetUserInvites_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	wsID := uuid.New()
	invID := uuid.New()
	const email = "user@example.com"

	q.EXPECT().GetUserInvitesByEmail(gomock.Any(), email).Return([]sqlc.GetUserInvitesByEmailRow{{
		ID:            invID,
		WorkspaceID:   wsID,
		Email:         email,
		Token:         "tok-xyz",
		Role:          "editor",
		ExpiresAt:     time.Now().Add(time.Hour),
		WorkspaceName: "Cool Workspace",
		WorkspaceSlug: "cool-ws",
	}}, nil)

	result, err := svc.GetUserInvites(context.Background(), email)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "Cool Workspace", result[0].WorkspaceName)
}

// --- GetUserWorkspaceRole ------------------------------------------------

func TestWorkspaceService_GetUserWorkspaceRole_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	wsID := uuid.New()
	userID := uuid.New()

	q.EXPECT().GetUserWorkspaceRole(gomock.Any(), sqlc.GetUserWorkspaceRoleParams{
		WorkspaceID: wsID, UserID: userID,
	}).Return("admin", nil)

	role, err := svc.GetUserWorkspaceRole(context.Background(), userID, wsID)
	require.NoError(t, err)
	assert.Equal(t, "admin", role)
}

// --- UpdateMemberRole ----------------------------------------------------

func TestWorkspaceService_UpdateMemberRole_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	wsID := uuid.New()
	userID := uuid.New()

	q.EXPECT().UpdateWorkspaceMemberRole(gomock.Any(), sqlc.UpdateWorkspaceMemberRoleParams{
		WorkspaceID: wsID, UserID: userID, Role: "admin",
	}).Return(nil)

	err := svc.UpdateMemberRole(context.Background(), wsID, userID, "admin")
	assert.NoError(t, err)
}

// --- DeleteWorkspaceInvite -----------------------------------------------

func TestWorkspaceService_DeleteWorkspaceInvite_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	repo := mocks.NewMockRepository(ctrl)
	svc := newWsSvc(q, repo)

	invID := uuid.New()
	q.EXPECT().DeleteWorkspaceInvite(gomock.Any(), invID).Return(nil)

	err := svc.DeleteWorkspaceInvite(context.Background(), invID)
	assert.NoError(t, err)
}
