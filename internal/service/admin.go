package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/repo"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"go.uber.org/zap"
)

type AdminService interface {
	ListUsers(ctx context.Context) ([]sqlc.ListUsersAdminRow, error)
	GetUser(ctx context.Context, userID uuid.UUID) (sqlc.GetUserAdminByIDRow, error)
	CreateUser(ctx context.Context, email string) (sqlc.User, error)
	DeleteUser(ctx context.Context, userID uuid.UUID) (sqlc.User, error)
	AssignUserPlan(ctx context.Context, userID, planID uuid.UUID) error
	ListPlans(ctx context.Context) ([]sqlc.Plan, error)
	CreatePlan(ctx context.Context, req CreatePlanRequest) (sqlc.Plan, error)
	UpdatePlan(ctx context.Context, id uuid.UUID, req CreatePlanRequest) error
	CapacitySummary(ctx context.Context) (sqlc.ServerCapacitySummaryRow, error)
}

type CreatePlanRequest struct {
	Name                          string `json:"name"`
	MaxTotalStorageBytes          int64  `json:"max_total_storage_bytes"`
	MaxFileSizeBytes              int64  `json:"max_file_size_bytes"`
	MaxFiles                      int32  `json:"max_files"`
	MaxFilesSentPerDay            int32  `json:"max_files_sent_per_day"`
	MaxSharesPerDay               int32  `json:"max_shares_per_day"`
	MaxFilesWorkspace             int64  `json:"max_files_workspace"`
	MaxUserWorkspaces             int64  `json:"max_user_workspaces"`
	MaxTotalStorageBytesWorkspace int64  `json:"max_total_storage_bytes_workspace"`
	MaxUsersWorkspace             int64  `json:"max_users_workspace"`
	MaxWorkspaceFolders           int64  `json:"max_workspace_folders"`
	MaxPrivateAPIKeys             int64  `json:"max_private_api_keys"`
	MaxWorkspaceAPIKeys           int64  `json:"max_workspace_api_keys"`
}

type adminService struct {
	log        *zap.SugaredLogger
	repository repo.Repository
}

func NewAdminService(log *zap.SugaredLogger, repository repo.Repository) AdminService {
	return &adminService{log: log, repository: repository}
}

func (s *adminService) ListUsers(ctx context.Context) ([]sqlc.ListUsersAdminRow, error) {
	return s.repository.Queries().ListUsersAdmin(ctx)
}

func (s *adminService) GetUser(ctx context.Context, userID uuid.UUID) (sqlc.GetUserAdminByIDRow, error) {
	return s.repository.Queries().GetUserAdminByID(ctx, userID)
}

func (s *adminService) CreateUser(ctx context.Context, email string) (sqlc.User, error) {
	return s.repository.Queries().CreateUser(ctx, email)
}

func (s *adminService) DeleteUser(ctx context.Context, userID uuid.UUID) (sqlc.User, error) {
	return s.repository.Queries().DeleteAccount(ctx, userID)
}

func (s *adminService) AssignUserPlan(ctx context.Context, userID, planID uuid.UUID) error {
	return s.repository.Queries().UpdateUserPlan(ctx, sqlc.UpdateUserPlanParams{ID: userID, PlanID: uuid.NullUUID{UUID: planID, Valid: true}})
}

func (s *adminService) ListPlans(ctx context.Context) ([]sqlc.Plan, error) {
	return s.repository.Queries().ListPlansAdmin(ctx)
}

func (s *adminService) CreatePlan(ctx context.Context, req CreatePlanRequest) (sqlc.Plan, error) {
	if req.Name == "" {
		return sqlc.Plan{}, fmt.Errorf("plan name is required")
	}
	return s.repository.Queries().CreatePlanAdmin(ctx, sqlc.CreatePlanAdminParams{
		Name:                          req.Name,
		MaxTotalStorageBytes:          req.MaxTotalStorageBytes,
		MaxFileSizeBytes:              req.MaxFileSizeBytes,
		MaxFiles:                      req.MaxFiles,
		MaxFilesSentPerDay:            req.MaxFilesSentPerDay,
		MaxSharesPerDay:               req.MaxSharesPerDay,
		MaxFilesWorkspace:             req.MaxFilesWorkspace,
		MaxUserWorkspaces:             req.MaxUserWorkspaces,
		MaxTotalStorageBytesWorkspace: req.MaxTotalStorageBytesWorkspace,
		MaxUsersWorkspace:             req.MaxUsersWorkspace,
		MaxWorkspaceFolders:           req.MaxWorkspaceFolders,
		MaxPrivateApiKeys:             req.MaxPrivateAPIKeys,
		MaxWorkspaceApiKeys:           req.MaxWorkspaceAPIKeys,
	})
}

func (s *adminService) UpdatePlan(ctx context.Context, id uuid.UUID, req CreatePlanRequest) error {
	if req.Name == "" {
		return fmt.Errorf("plan name is required")
	}
	return s.repository.Queries().UpdatePlanAdmin(ctx, sqlc.UpdatePlanAdminParams{
		Name:                          req.Name,
		MaxTotalStorageBytes:          req.MaxTotalStorageBytes,
		MaxFileSizeBytes:              req.MaxFileSizeBytes,
		MaxFiles:                      req.MaxFiles,
		MaxFilesSentPerDay:            req.MaxFilesSentPerDay,
		MaxSharesPerDay:               req.MaxSharesPerDay,
		MaxFilesWorkspace:             req.MaxFilesWorkspace,
		MaxUserWorkspaces:             req.MaxUserWorkspaces,
		MaxTotalStorageBytesWorkspace: req.MaxTotalStorageBytesWorkspace,
		MaxUsersWorkspace:             req.MaxUsersWorkspace,
		MaxWorkspaceFolders:           req.MaxWorkspaceFolders,
		MaxPrivateApiKeys:             req.MaxPrivateAPIKeys,
		MaxWorkspaceApiKeys:           req.MaxWorkspaceAPIKeys,
		ID:                            id,
	})
}

func (s *adminService) CapacitySummary(ctx context.Context) (sqlc.ServerCapacitySummaryRow, error) {
	return s.repository.Queries().ServerCapacitySummary(ctx)
}
