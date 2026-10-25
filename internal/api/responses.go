package api

import (
	"time"

	"github.com/tscrond/fluxsend-backend/internal/mappings"
	"github.com/tscrond/fluxsend-backend/internal/service"
	"github.com/tscrond/fluxsend-backend/internal/userdata"
)

// APIResponse is the JSON envelope written by pkg.WriteJSONResponse.
type APIResponse[T any] struct {
	Status   int    `json:"status" example:"200"`
	Msg      string `json:"msg,omitempty"`
	Response T      `json:"response"`
}

// ErrorResponse documents the envelope for failure paths.
type ErrorResponse struct {
	Status   int    `json:"status" example:"400"`
	Msg      string `json:"msg,omitempty"`
	Response any    `json:"response"`
}

type APIKeyCreatedResponseData struct {
	APIKey service.CreateAPIKeyResult `json:"api_key"`
}

type APIKeyListResponseData struct {
	APIKeys []service.APIKeyResult `json:"api_keys"`
}

type AccountDeletedResponseData struct {
	Bucket         DeletedBucket       `json:"bucket"`
	AccountDeleted DeletedAccountOwner `json:"account_deleted"`
}

type DeletedBucket struct {
	Name    string `json:"name"`
	Deleted bool   `json:"deleted"`
}

type DeletedAccountOwner struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	UserName string `json:"user_name"`
}

type UserDataResponseData struct {
	UserData userdata.AuthorizedUserWithPlan `json:"user_data"`
}

type UserBucketResponseData struct {
	BucketData *mappings.BucketData `json:"bucket_data"`
}

type IdentityResponse struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	Provider       string    `json:"provider"`
	ProviderUserID string    `json:"provider_user_id"`
	Email          string    `json:"email"`
	EmailVerified  bool      `json:"email_verified"`
	Name           string    `json:"name"`
	AvatarURL      string    `json:"avatar_url"`
	CreatedAt      time.Time `json:"created_at"`
}

type UserIdentitiesResponseData struct {
	Identities          []IdentityResponse `json:"identities"`
	HasPasswordIdentity bool               `json:"has_password_identity"`
}

type PrivateDownloadTokenResponseData struct {
	PrivateDownloadToken string `json:"private_download_token"`
}

type DailyActivity struct {
	Day     time.Time `json:"day"`
	Uploads int64     `json:"uploads,omitempty"`
	Shares  int64     `json:"shares,omitempty"`
}

type UserStatsResponseData struct {
	TotalFiles       int64           `json:"total_files"`
	TotalBytes       int64           `json:"total_bytes"`
	FilesToday       int64           `json:"files_today"`
	FilesLast7d      int64           `json:"files_last_7d"`
	FilesLast30d     int64           `json:"files_last_30d"`
	TotalSharesSent  int64           `json:"total_shares_sent"`
	SharesToday      int64           `json:"shares_today"`
	TargetedShares   int64           `json:"targeted_shares"`
	PublicShares     int64           `json:"public_shares"`
	ActiveShares     int64           `json:"active_shares"`
	TotalReceived    int64           `json:"total_received"`
	OwnedWorkspaces  int64           `json:"owned_workspaces"`
	DailyUploads     []DailyActivity `json:"daily_uploads"`
	DailyShares      []DailyActivity `json:"daily_shares"`
	WorkspaceAPIKeys int64           `json:"workspace_api_keys"`
}

type FileDeleteResponseData struct {
	FileDeleted string `json:"file_deleted"`
}

type FilesDeleteResponseData struct {
	FilesDeleted []string `json:"files_deleted"`
	FilesFailed  []string `json:"files_failed"`
}

type FoldersResponseData struct {
	Path    string   `json:"path"`
	Folders []string `json:"folders"`
}

type FolderDeleteResponseData struct {
	FolderDeleted string `json:"folder_deleted"`
	FilesDeleted  int    `json:"files_deleted"`
}

type FileMoveResponseData struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

type FolderMoveResponseData struct {
	FileMoveResponseData
	FilesMoved int `json:"files_moved"`
}

type NoteResponseData struct {
	Note string `json:"note"`
}

type NoteContentResponseData struct {
	Content string `json:"content"`
}

type UnseenSharesResponseData struct {
	Count int64 `json:"count"`
}

type ShareResponseData struct {
	SharingInfo        []service.ShareResult `json:"sharing_info"`
	NotificationStatus string                `json:"notification_status"`
}

type SharedFilesResponseData struct {
	Files []service.SharedFileInfo `json:"files"`
}

type ShareResolveResponseData struct {
	URL      string `json:"url"`
	FileName string `json:"file_name"`
}

type ShareRevokeResponseData struct {
	TokenRevoked string `json:"token_revoked"`
}

type WorkspaceCreatedResponseData struct {
	Workspace WorkspaceIdentity `json:"workspace"`
}

type WorkspaceIdentity struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type WorkspaceDeletedResponseData struct {
	WorkspaceID string `json:"workspace_id"`
}

type WorkspaceInviteDeletedResponseData struct {
	InviteID string `json:"invite_id"`
}

type WorkspaceMemberRoleResponseData struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

type WorkspaceQuotaResponseData struct {
	FileCount                     int64 `json:"file_count"`
	TotalBytes                    int64 `json:"total_bytes"`
	FolderCount                   int64 `json:"folder_count"`
	MemberCount                   int64 `json:"member_count"`
	APIKeyCount                   int64 `json:"api_key_count"`
	MaxFilesWorkspace             int64 `json:"max_files_workspace"`
	MaxTotalStorageBytesWorkspace int64 `json:"max_total_storage_bytes_workspace"`
	MaxUsersWorkspace             int64 `json:"max_users_workspace"`
	MaxWorkspaceFolders           int64 `json:"max_workspace_folders"`
	MaxWorkspaceAPIKeys           int64 `json:"max_workspace_api_keys"`
}

type WorkspaceFilesMoveResponseData struct {
	Updated int `json:"updated"`
}

type WorkspaceRenameResponse = APIResponse[service.WorkspaceResult]

type WorkspaceInviteCreatedResponse = APIResponse[service.WorkspaceInviteResult]
