package scope

// Scope is a single API key capability string.
type Scope string

const (
	PrivateFilesRead   Scope = "private_files:read"
	PrivateFilesWrite  Scope = "private_files:write"
	PrivateFilesDelete Scope = "private_files:delete"
	PrivateFilesShare  Scope = "private_files:share"

	WorkspacesRead          Scope = "workspaces:read"
	WorkspacesWrite         Scope = "workspaces:write"
	WorkspacesDelete        Scope = "workspaces:delete"
	WorkspacesMembersRead   Scope = "workspaces:members:read"
	WorkspacesMembersManage Scope = "workspaces:members:manage"
	WorkspacesInvitesManage Scope = "workspaces:invites:manage"
	WorkspacesFilesRead     Scope = "workspaces:files:read"
	WorkspacesFilesWrite    Scope = "workspaces:files:write"
	WorkspacesFilesDelete   Scope = "workspaces:files:delete"
)

func (s Scope) String() string {
	return string(s)
}

var allScopes = []Scope{
	PrivateFilesRead,
	PrivateFilesWrite,
	PrivateFilesDelete,
	PrivateFilesShare,
	WorkspacesRead,
	WorkspacesWrite,
	WorkspacesDelete,
	WorkspacesMembersRead,
	WorkspacesMembersManage,
	WorkspacesInvitesManage,
	WorkspacesFilesRead,
	WorkspacesFilesWrite,
	WorkspacesFilesDelete,
}

var privateScopes = []Scope{
	PrivateFilesRead,
	PrivateFilesWrite,
	PrivateFilesDelete,
	PrivateFilesShare,
}

var workspaceScopes = []Scope{
	WorkspacesRead,
	WorkspacesWrite,
	WorkspacesDelete,
	WorkspacesMembersRead,
	WorkspacesMembersManage,
	WorkspacesInvitesManage,
	WorkspacesFilesRead,
	WorkspacesFilesWrite,
	WorkspacesFilesDelete,
}

func All() []Scope {
	out := make([]Scope, len(allScopes))
	copy(out, allScopes)
	return out
}

func Private() []Scope {
	out := make([]Scope, len(privateScopes))
	copy(out, privateScopes)
	return out
}

func Workspace() []Scope {
	out := make([]Scope, len(workspaceScopes))
	copy(out, workspaceScopes)
	return out
}

func IsKnown(scopeValue string) bool {
	for _, s := range allScopes {
		if s.String() == scopeValue {
			return true
		}
	}
	return false
}

func ToSet(scopes []Scope) map[string]struct{} {
	set := make(map[string]struct{}, len(scopes))
	for _, s := range scopes {
		set[s.String()] = struct{}{}
	}
	return set
}
