package api

type PrivateAPIKeyCreateRequest struct {
	Name        string   `json:"name" binding:"required" example:"ci-deploy"`
	Description string   `json:"description" example:"CI deployment key"`
	Scopes      []string `json:"scopes" binding:"required" example:"private_files:read,private_files:write" enums:"private_files:read,private_files:write,private_files:delete,private_files:share"`
}

type WorkspaceAPIKeyCreateRequest struct {
	Name        string   `json:"name" binding:"required" example:"ci-deploy"`
	Description string   `json:"description" example:"CI deployment key"`
	Scopes      []string `json:"scopes" binding:"required" example:"workspaces:read,workspaces:files:read" enums:"workspaces:read,workspaces:write,workspaces:delete,workspaces:members:read,workspaces:members:manage,workspaces:invites:manage,workspaces:files:read,workspaces:files:write,workspaces:files:delete"`
}

type APIKeyDeleteRequest struct {
	APIKeyID string `json:"api_key_id" binding:"required" example:"550e8400-e29b-41d4-a716-446655440000" format:"uuid"`
}

type FileBatchDeleteRequest struct {
	Files []string `json:"files" binding:"required" example:"movies/m1.mov,photos/p1.jpg,photos2/photo.png"`
}

type MoveRequest struct {
	Source      string `json:"source" binding:"required" example:"documents/old-name.pdf"`
	Destination string `json:"destination" binding:"required" example:"archive/new-name.pdf"`
}

type NoteContentRequest struct {
	Content string `json:"content" maxLength:"500" example:"Quarterly report"`
}

type QuickShareRequest struct {
	Object   string `json:"object" binding:"required" example:"documents/report.pdf"`
	Duration string `json:"duration" example:"24h"`
	Password string `json:"password" example:"share-secret"`
}

type MarkSeenRequest struct {
	SharingToken string `json:"sharing_token" binding:"required" example:"share-token"`
}

type ShareRequest struct {
	Email     string   `json:"email" binding:"required" format:"email" example:"colleague@example.com"`
	Objects   []string `json:"objects" binding:"required" example:"documents/report.pdf,images/chart.png"`
	Duration  string   `json:"duration" binding:"required" example:"24h"`
	Password  string   `json:"password" example:"share-secret"`
	SendEmail bool     `json:"send_email" example:"true"`
}

type SharePasswordRequest struct {
	Password string `json:"password" example:"share-secret"`
}

type DeleteAccountRequest struct {
	DeleteUserData bool `json:"delete_user_data" example:"true"`
}

type WorkspaceCreateRequest struct {
	Name string `json:"name" binding:"required" maxLength:"64" example:"Platform Team"`
	Slug string `json:"slug" binding:"required" maxLength:"48" example:"platform-team"`
}

type WorkspaceInviteCreateRequest struct {
	WorkspaceID string `json:"workspace_id" binding:"required" format:"uuid" example:"550e8400-e29b-41d4-a716-446655440000"`
	Email       string `json:"email" binding:"required" format:"email" example:"colleague@example.com"`
	Role        string `json:"role" binding:"required" enums:"admin,editor,viewer" example:"editor"`
}

type WorkspaceInviteAcceptRequest struct {
	Token string `json:"token" binding:"required" example:"invite-token"`
}

type WorkspaceRenameRequest struct {
	WorkspaceID string `json:"workspace_id" binding:"required" format:"uuid" example:"550e8400-e29b-41d4-a716-446655440000"`
	Name        string `json:"name" binding:"required" maxLength:"64" example:"Renamed Workspace"`
	Slug        string `json:"slug" maxLength:"48" example:"renamed-workspace"`
}

type WorkspaceMemberRoleRequest struct {
	WorkspaceID string `json:"workspace_id" binding:"required" format:"uuid" example:"550e8400-e29b-41d4-a716-446655440000"`
	UserID      string `json:"user_id" binding:"required" format:"uuid" example:"0f8fad5b-d9cb-469f-a165-70867728950e"`
	Role        string `json:"role" binding:"required" enums:"owner,admin,editor,viewer" example:"editor"`
}

type WorkspaceFileMoveRequest struct {
	FileID      string `json:"file_id" binding:"required" format:"uuid" example:"0f8fad5b-d9cb-469f-a165-70867728950e"`
	Destination string `json:"destination" binding:"required" example:"archive/report.pdf"`
}

type WorkspaceFolderCreateRequest struct {
	FolderName string `json:"folder_name" binding:"required" example:"reports"`
	ParentPath string `json:"parent_path" example:"documents"`
}
