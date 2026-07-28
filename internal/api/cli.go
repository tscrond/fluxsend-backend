package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger"
	"github.com/tscrond/fluxsend-backend/docs"
	"github.com/tscrond/fluxsend-backend/internal/config"
	chimiddleware "github.com/tscrond/fluxsend-backend/internal/middleware"
	"github.com/tscrond/fluxsend-backend/internal/scope"
)

type CLIServer struct {
	*CoreHandlers
	routePrefix string
}

type CLIServerDependencies struct {
	CoreHandlersDependencies
}

func NewCLIServer(backendConfig config.BackendConfig, routePrefix string, deps CLIServerDependencies) *CLIServer {
	return &CLIServer{
		CoreHandlers: NewCoreHandlers(backendConfig, deps.CoreHandlersDependencies),
		routePrefix:  routePrefix,
	}
}

func (s *CLIServer) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestLogger(s.log))

	r.Route(s.routePrefix, func(r chi.Router) {
		r.Get("/swagger/json", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(docs.SwaggerInfo.ReadDoc()))
		})
		r.Get("/swagger/*", httpSwagger.Handler(
			httpSwagger.URL("/api/swagger/json"),
		))
		s.registerCLIFileRoutes(r)
		s.registerCLIWorkspaceRoutes(r)
		s.registerCLISharingRoutes(r)
		s.registerCLIUserRoutes(r)
		s.registerCLIAPIKeyRoutes(r)
	})

	return r
}

func (s *CLIServer) Start() {
	s.log.Infof("CLI server listening on %s", s.backendConfig.ListenPort)
	http.ListenAndServe("0.0.0.0"+s.backendConfig.ListenPort, s.Handler())
}

func chainRouteMiddleware(middlewares ...routeMiddleware) routeMiddleware {
	return func(next http.Handler) http.Handler {
		wrapped := next
		for i := len(middlewares) - 1; i >= 0; i-- {
			if middlewares[i] == nil {
				continue
			}
			wrapped = middlewares[i](wrapped)
		}
		return wrapped
	}
}

func (s *CLIServer) cliProtected(requiredScope scope.Scope, domain routeDomain, requiresWorkspaceID bool) routeMiddleware {
	return chainRouteMiddleware(
		s.authMiddleware,
		s.requireScope(requiredScope),
		s.requireKeyBinding(domain, requiresWorkspaceID),
	)
}

func (s *CLIServer) registerCLIFileRoutes(r chi.Router) {
	privateRead := s.cliProtected(scope.PrivateFilesRead, routeDomainPrivate, false)
	privateWrite := s.cliProtected(scope.PrivateFilesWrite, routeDomainPrivate, false)
	privateDelete := s.cliProtected(scope.PrivateFilesDelete, routeDomainPrivate, false)
	privateShare := s.cliProtected(scope.PrivateFilesShare, routeDomainPrivate, false)

	r.Handle("/files/upload", applyRouteMiddleware(s.uploadHandler, privateWrite))
	r.Handle("/files/uploads", applyRouteMiddleware(s.uploadInitHandler, privateWrite))
	r.Handle("/files/uploads/{upload_id}", applyRouteMiddleware(s.abortUploadHandler, privateWrite))
	r.Handle("/files/uploads/{upload_id}/parts/{part_id}", applyRouteMiddleware(s.uploadPartHandler, privateWrite))
	r.Handle("/files/uploads/{upload_id}/complete", applyRouteMiddleware(s.completeUploadHandler, privateWrite))
	r.Handle("/files/share", applyRouteMiddleware(s.shareWith, privateShare))
	r.Handle("/files/tree", applyRouteMiddleware(s.getFilesTree, privateRead))
	r.Handle("/files/move", applyRouteMiddleware(s.moveFile, privateWrite))
	r.Handle("/files/received", applyRouteMiddleware(s.getDataSharedForUser, privateRead))
	r.Handle("/files/received/unseen_count", applyRouteMiddleware(s.getUnseenReceivedCount, privateRead))
	r.Handle("/files/received/mark_seen", applyRouteMiddleware(s.markReceivedSeen, privateWrite))
	r.Handle("/files/shared_by_user", applyRouteMiddleware(s.getDataSharedByUser, privateRead))
	r.Handle("/files/quick_share", applyRouteMiddleware(s.quickShare, privateShare))
	r.Handle("/files/delete", applyRouteMiddleware(s.deleteFile, privateDelete))
	r.Handle("/files/delete/batch", applyRouteMiddleware(s.deleteFilesBatch, privateDelete))
	r.Get("/files/{checksum}/note", applyRouteMiddleware(s.getFileNotes, privateRead).ServeHTTP)
	r.Put("/files/{checksum}/note", applyRouteMiddleware(s.editFileNotes, privateWrite).ServeHTTP)

	r.Get("/folders", applyRouteMiddleware(s.getFolders, privateRead).ServeHTTP)
	r.Delete("/folders", applyRouteMiddleware(s.deleteFolder, privateDelete).ServeHTTP)
	r.Handle("/folders/move", applyRouteMiddleware(s.moveFolder, privateWrite))
}

func (s *CLIServer) registerCLIWorkspaceRoutes(r chi.Router) {
	workspaceRead := s.cliProtected(scope.WorkspacesRead, routeDomainWorkspace, true)
	workspaceWrite := s.cliProtected(scope.WorkspacesWrite, routeDomainWorkspace, true)
	workspaceDelete := s.cliProtected(scope.WorkspacesDelete, routeDomainWorkspace, true)
	workspaceMembersRead := s.cliProtected(scope.WorkspacesMembersRead, routeDomainWorkspace, true)
	workspaceMembersManage := s.cliProtected(scope.WorkspacesMembersManage, routeDomainWorkspace, true)
	workspaceInvitesManage := s.cliProtected(scope.WorkspacesInvitesManage, routeDomainWorkspace, true)
	workspaceFilesRead := s.cliProtected(scope.WorkspacesFilesRead, routeDomainWorkspace, true)
	workspaceFilesWrite := s.cliProtected(scope.WorkspacesFilesWrite, routeDomainWorkspace, true)
	workspaceFilesDelete := s.cliProtected(scope.WorkspacesFilesDelete, routeDomainWorkspace, true)

	r.Handle("/workspaces/create", applyRouteMiddleware(s.createWorkspace, workspaceWrite))
	r.Handle("/workspaces/list", applyRouteMiddleware(s.listWorkspaces, workspaceRead))
	r.Handle("/workspaces/members", applyRouteMiddleware(s.getWorkspaceMembers, workspaceMembersRead))
	r.Handle("/workspaces/members/remove", applyRouteMiddleware(s.removeWorkspaceMember, workspaceMembersManage))
	r.Handle("/workspaces/invites", applyRouteMiddleware(s.getWorkspaceInvites, workspaceInvitesManage))
	r.Handle("/workspaces/invites/mine", applyRouteMiddleware(s.getMyWorkspaceInvites, workspaceInvitesManage))
	r.Handle("/workspaces/invites/create", applyRouteMiddleware(s.createWorkspaceInvite, workspaceInvitesManage))
	r.Handle("/workspaces/invites/accept", applyRouteMiddleware(s.acceptWorkspaceInvite, workspaceInvitesManage))
	r.Handle("/workspaces/invites/reject", applyRouteMiddleware(s.rejectWorkspaceInvite, workspaceInvitesManage))
	r.Handle("/workspaces/invites/delete", applyRouteMiddleware(s.deleteWorkspaceInvite, workspaceInvitesManage))
	r.Handle("/workspaces/delete", applyRouteMiddleware(s.deleteWorkspace, workspaceDelete))
	r.Handle("/workspaces/rename", applyRouteMiddleware(s.renameWorkspace, workspaceWrite))
	r.Handle("/workspaces/members/role", applyRouteMiddleware(s.changeMemberRole, workspaceMembersManage))

	r.Handle("/workspaces/{workspace_id}/files/tree", applyRouteMiddleware(s.getWorkspaceFilesTree, workspaceFilesRead))
	r.Handle("/workspaces/{workspace_id}/files/upload", applyRouteMiddleware(s.uploadWorkspaceFile, workspaceFilesWrite))
	r.Handle("/workspaces/{workspace_id}/files/uploads", applyRouteMiddleware(s.initWorkspaceUploadHandler, workspaceFilesWrite))
	r.Handle("/workspaces/{workspace_id}/files/uploads/{upload_id}", applyRouteMiddleware(s.abortWorkspaceUploadHandler, workspaceFilesWrite))
	r.Handle("/workspaces/{workspace_id}/files/uploads/{upload_id}/parts/{part_id}", applyRouteMiddleware(s.uploadWorkspacePartHandler, workspaceFilesWrite))
	r.Handle("/workspaces/{workspace_id}/files/uploads/{upload_id}/complete", applyRouteMiddleware(s.completeWorkspaceUploadHandler, workspaceFilesWrite))
	r.Handle("/workspaces/{workspace_id}/files/mkdir", applyRouteMiddleware(s.mkdirWorkspace, workspaceFilesWrite))
	r.Handle("/workspaces/{workspace_id}/files/delete", applyRouteMiddleware(s.deleteWorkspaceFile, workspaceFilesDelete))
	r.Handle("/workspaces/{workspace_id}/files/folder/delete", applyRouteMiddleware(s.deleteWorkspaceFolder, workspaceFilesDelete))
	r.Handle("/workspaces/{workspace_id}/files/move", applyRouteMiddleware(s.moveWorkspaceFile, workspaceFilesWrite))
	r.Handle("/workspaces/{workspace_id}/files/folder/move", applyRouteMiddleware(s.moveWorkspaceFolder, workspaceFilesWrite))
	r.Handle("/workspaces/{workspace_id}/files/download", applyRouteMiddleware(s.downloadWorkspaceFile, workspaceFilesRead))
	r.Handle("/workspaces/{workspace_id}/quota", applyRouteMiddleware(s.getWorkspaceQuota, workspaceRead))
}

func (s *CLIServer) registerCLISharingRoutes(r chi.Router) {
	privateRead := s.cliProtected(scope.PrivateFilesRead, routeDomainPrivate, false)
	privateShare := s.cliProtected(scope.PrivateFilesShare, routeDomainPrivate, false)

	r.Handle("/d/private/{token}", applyRouteMiddleware(s.downloadThroughProxyPersonal, privateRead))
	r.Handle("/d/{token}", applyRouteMiddleware(s.downloadThroughProxy, nil))
	r.Handle("/share/info/{token}", applyRouteMiddleware(s.publicShareInfo, nil))
	r.Handle("/share/resolve/{token}", applyRouteMiddleware(s.resolvePublicShare, nil))
	r.Handle("/share/revoke/{token}", applyRouteMiddleware(s.revokeShare, privateShare))
}

func (s *CLIServer) registerCLIUserRoutes(r chi.Router) {
	privateRead := s.cliProtected(scope.PrivateFilesRead, routeDomainPrivate, false)
	privateDelete := s.cliProtected(scope.PrivateFilesDelete, routeDomainPrivate, false)

	r.Handle("/user/data", applyRouteMiddleware(s.getUserData, privateRead))
	r.Handle("/user/bucket", applyRouteMiddleware(s.getUserBucketData, privateRead))
	r.Handle("/user/private/download_token", applyRouteMiddleware(s.getUserPrivateFileByName, privateRead))
	r.Handle("/user/account/delete", applyRouteMiddleware(s.deleteAccount, privateDelete))
	r.Handle("/user/stats", applyRouteMiddleware(s.getUserStats, privateRead))
}

func (s *CLIServer) registerCLIAPIKeyRoutes(r chi.Router) {
	privateRead := s.cliProtected(scope.PrivateFilesRead, routeDomainPrivate, false)
	privateWrite := s.cliProtected(scope.PrivateFilesWrite, routeDomainPrivate, false)
	privateDelete := s.cliProtected(scope.PrivateFilesDelete, routeDomainPrivate, false)
	workspaceRead := s.cliProtected(scope.WorkspacesRead, routeDomainWorkspace, true)
	workspaceWrite := s.cliProtected(scope.WorkspacesWrite, routeDomainWorkspace, true)
	workspaceDelete := s.cliProtected(scope.WorkspacesDelete, routeDomainWorkspace, true)

	r.Handle("/api_keys/{workspace_id}/create", applyRouteMiddleware(s.createWorkspaceAPIKey, workspaceWrite))
	r.Handle("/api_keys/{workspace_id}/list", applyRouteMiddleware(s.listWorkspaceAPIKeys, workspaceRead))
	r.Handle("/api_keys/{workspace_id}/delete", applyRouteMiddleware(s.deleteWorkspaceAPIKey, workspaceDelete))
	r.Handle("/api_keys/private/create", applyRouteMiddleware(s.createPrivateAPIKey, privateWrite))
	r.Handle("/api_keys/private/list", applyRouteMiddleware(s.listPrivateAPIKeys, privateRead))
	r.Handle("/api_keys/private/delete", applyRouteMiddleware(s.deletePrivateAPIKey, privateDelete))
}
