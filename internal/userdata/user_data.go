package userdata

type planContextKey struct{}
type authorizedUserContextKey struct{}
type verifiedUserContextKey struct{}
type authorizedCLIUserWithPlanContextKey struct{}

var VerifiedUserContextKey = verifiedUserContextKey{}
var AuthorizedUserContextKey = authorizedUserContextKey{}
var AuthorizedUserWithPlanContextKey = planContextKey{}
var AuthorizedCLIUserWithPlanContextKey = authorizedCLIUserWithPlanContextKey{}

type AuthorizedUserWithPlan struct {
	AuthorizedUserInfo
	UserPlan
}

type AuthorizedCLIUserWithPlan struct {
	AuthorizedCLIUserInfo
	UserPlan
}

type UserPlan struct {
	PlanID                        string `json:"plan_id"`
	PlanName                      string `json:"plan_name"`
	MaxFileSizeBytes              int64  `json:"max_file_size_bytes"`
	MaxTotalStorageBytes          int64  `json:"max_total_storage_bytes"`
	MaxFiles                      int32  `json:"max_files"`
	MaxFilesSentPerDay            int32  `json:"max_files_sent_per_day"`
	MaxSharesPerDay               int32  `json:"max_shares_per_day"`
	MaxFilesWorkspace             int64  `json:"max_files_workspace"`
	MaxUserWorkspaces             int64  `json:"max_user_workspaces"`
	MaxWorkspaceFolders           int64  `json:"max_workspace_folders"`
	MaxUsersPerWorkspace          int64  `json:"max_users_workspace"`
	MaxTotalStorageBytesWorkspace int64  `json:"max_total_storage_bytes_workspace"`
	MaxPrivateAPIKeys             int64  `json:"max_private_api_keys"`
	MaxWorkspaceAPIKeys           int64  `json:"max_workspace_api_keys"`
}

// Info gathered after successful oauth2 callback
type AuthorizedUserInfo struct {
	InternalID string `json:"internal_id"` // UUID from our database
	Id         string `json:"id"`          // OAuth subject ID
	Email      string `json:"email"`
	Name       string `json:"name"`
	GivenName  string `json:"given_name"`
	FamilyName string `json:"family_name"`
	Picture    string `json:"picture"`
	Locale     string `json:"locale"`
	Provider   string `json:"provider"`
}

type AuthorizedCLIUserInfo struct {
	InternalID string `json:"internal_id"`
	Email      string `json:"email"`
	Name       string `json:"name"`
}
