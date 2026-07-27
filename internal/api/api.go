package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/cors"
	"github.com/tscrond/fluxsend-backend/internal/auth"
	"github.com/tscrond/fluxsend-backend/internal/cdn"
	storagetypes "github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	"github.com/tscrond/fluxsend-backend/internal/config"
	mailtypes "github.com/tscrond/fluxsend-backend/internal/mailservice/types"
	chimiddleware "github.com/tscrond/fluxsend-backend/internal/middleware"
	"github.com/tscrond/fluxsend-backend/internal/repo"
	"github.com/tscrond/fluxsend-backend/internal/service"
	"github.com/tscrond/fluxsend-backend/internal/tokencrypto"
	"go.uber.org/zap"
)

type CoreHandlers struct {
	log              *zap.SugaredLogger
	backendConfig    config.BackendConfig
	bucketHandler    storagetypes.ObjectStorage
	cloudFrontSigner *cdn.CloudFrontURLSigner
	emailSender      mailtypes.EmailSender
	repository       repo.Repository

	files          service.FileService
	shares         service.ShareService
	users          service.UserService
	workspaces     service.WorkspaceService
	workspaceFiles service.WorkspaceFileService
	apiKeys        service.APIKeyService
}

type APIServer struct {
	*CoreHandlers
	authProviders  map[string]auth.AuthProvider
	tokenEncryptor *tokencrypto.Encryptor
}

type routeMiddleware func(http.Handler) http.Handler

type CoreHandlersDependencies struct {
	Log              *zap.SugaredLogger
	EmailSender      mailtypes.EmailSender
	BucketHandler    storagetypes.ObjectStorage
	CloudFrontSigner *cdn.CloudFrontURLSigner
	Repository       repo.Repository
	Files            service.FileService
	Shares           service.ShareService
	Users            service.UserService
	Workspaces       service.WorkspaceService
	WorkspaceFiles   service.WorkspaceFileService
	ApiKeys          service.APIKeyService
}

type APIServerDependencies struct {
	CoreHandlersDependencies
	AuthProviders  map[string]auth.AuthProvider
	TokenEncryptor *tokencrypto.Encryptor
}

func NewCoreHandlers(backendConfig config.BackendConfig, deps CoreHandlersDependencies) *CoreHandlers {
	return &CoreHandlers{
		log:              deps.Log,
		backendConfig:    backendConfig,
		bucketHandler:    deps.BucketHandler,
		cloudFrontSigner: deps.CloudFrontSigner,
		emailSender:      deps.EmailSender,
		repository:       deps.Repository,
		files:            deps.Files,
		shares:           deps.Shares,
		users:            deps.Users,
		workspaces:       deps.Workspaces,
		workspaceFiles:   deps.WorkspaceFiles,
		apiKeys:          deps.ApiKeys,
	}
}

func applyRouteMiddleware(handler http.HandlerFunc, middleware routeMiddleware) http.Handler {
	if middleware == nil {
		return handler
	}
	return middleware(handler)
}

func (s *CoreHandlers) registerFileRoutes(r chi.Router, protected routeMiddleware) {
	r.Handle("/files/upload", applyRouteMiddleware(s.uploadHandler, protected))
	r.Handle("/files/uploads", applyRouteMiddleware(s.uploadInitHandler, protected))
	r.Handle("/files/uploads/{upload_id}", applyRouteMiddleware(s.abortUploadHandler, protected))
	r.Handle("/files/uploads/{upload_id}/parts/{part_id}", applyRouteMiddleware(s.uploadPartHandler, protected))
	r.Handle("/files/uploads/{upload_id}/complete", applyRouteMiddleware(s.completeUploadHandler, protected))
	r.Handle("/files/share", applyRouteMiddleware(s.shareWith, protected))
	r.Handle("/files/tree", applyRouteMiddleware(s.getFilesTree, protected))
	r.Handle("/files/move", applyRouteMiddleware(s.moveFile, protected))

	r.Handle("/files/received", applyRouteMiddleware(s.getDataSharedForUser, protected))
	r.Handle("/files/received/unseen_count", applyRouteMiddleware(s.getUnseenReceivedCount, protected))
	r.Handle("/files/received/mark_seen", applyRouteMiddleware(s.markReceivedSeen, protected))
	r.Handle("/files/shared_by_user", applyRouteMiddleware(s.getDataSharedByUser, protected))
	r.Handle("/files/quick_share", applyRouteMiddleware(s.quickShare, protected))
	r.Handle("/files/delete", applyRouteMiddleware(s.deleteFile, protected))
	r.Handle("/files/delete/batch", applyRouteMiddleware(s.deleteFilesBatch, protected))
	r.Handle("/files/{checksum}/note", applyRouteMiddleware(s.fileNotesHandler, protected))

	r.Handle("/folders", applyRouteMiddleware(s.foldersHandler, protected))
	r.Handle("/folders/move", applyRouteMiddleware(s.moveFolder, protected))
}

func (s *CoreHandlers) registerWorkspaceRoutes(r chi.Router, protected routeMiddleware) {
	r.Handle("/workspaces/create", applyRouteMiddleware(s.createWorkspace, protected))
	r.Handle("/workspaces/list", applyRouteMiddleware(s.listWorkspaces, protected))
	r.Handle("/workspaces/members", applyRouteMiddleware(s.getWorkspaceMembers, protected))
	r.Handle("/workspaces/members/remove", applyRouteMiddleware(s.removeWorkspaceMember, protected))
	r.Handle("/workspaces/invites", applyRouteMiddleware(s.getWorkspaceInvites, protected))
	r.Handle("/workspaces/invites/mine", applyRouteMiddleware(s.getMyWorkspaceInvites, protected))
	r.Handle("/workspaces/invites/create", applyRouteMiddleware(s.createWorkspaceInvite, protected))
	r.Handle("/workspaces/invites/accept", applyRouteMiddleware(s.acceptWorkspaceInvite, protected))
	r.Handle("/workspaces/invites/reject", applyRouteMiddleware(s.rejectWorkspaceInvite, protected))
	r.Handle("/workspaces/invites/delete", applyRouteMiddleware(s.deleteWorkspaceInvite, protected))
	r.Handle("/workspaces/delete", applyRouteMiddleware(s.deleteWorkspace, protected))
	r.Handle("/workspaces/rename", applyRouteMiddleware(s.renameWorkspace, protected))
	r.Handle("/workspaces/members/role", applyRouteMiddleware(s.changeMemberRole, protected))

	r.Handle("/workspaces/{workspace_id}/files/tree", applyRouteMiddleware(s.getWorkspaceFilesTree, protected))
	r.Handle("/workspaces/{workspace_id}/files/upload", applyRouteMiddleware(s.uploadWorkspaceFile, protected))
	r.Handle("/workspaces/{workspace_id}/files/uploads", applyRouteMiddleware(s.initWorkspaceUploadHandler, protected))
	r.Handle("/workspaces/{workspace_id}/files/uploads/{upload_id}", applyRouteMiddleware(s.abortWorkspaceUploadHandler, protected))
	r.Handle("/workspaces/{workspace_id}/files/uploads/{upload_id}/parts/{part_id}", applyRouteMiddleware(s.uploadWorkspacePartHandler, protected))
	r.Handle("/workspaces/{workspace_id}/files/uploads/{upload_id}/complete", applyRouteMiddleware(s.completeWorkspaceUploadHandler, protected))
	r.Handle("/workspaces/{workspace_id}/files/mkdir", applyRouteMiddleware(s.mkdirWorkspace, protected))
	r.Handle("/workspaces/{workspace_id}/files/delete", applyRouteMiddleware(s.deleteWorkspaceFile, protected))
	r.Handle("/workspaces/{workspace_id}/files/folder/delete", applyRouteMiddleware(s.deleteWorkspaceFolder, protected))
	r.Handle("/workspaces/{workspace_id}/files/move", applyRouteMiddleware(s.moveWorkspaceFile, protected))
	r.Handle("/workspaces/{workspace_id}/files/folder/move", applyRouteMiddleware(s.moveWorkspaceFolder, protected))
	r.Handle("/workspaces/{workspace_id}/files/download", applyRouteMiddleware(s.downloadWorkspaceFile, protected))
	r.Handle("/workspaces/{workspace_id}/quota", applyRouteMiddleware(s.getWorkspaceQuota, protected))
}

func (s *CoreHandlers) registerSharingRoutes(r chi.Router, protected routeMiddleware) {
	r.Handle("/d/private/{token}", applyRouteMiddleware(s.downloadThroughProxyPersonal, protected))
	r.Handle("/d/{token}", applyRouteMiddleware(s.downloadThroughProxy, nil))
	r.Handle("/share/info/{token}", applyRouteMiddleware(s.publicShareInfo, nil))
	r.Handle("/share/resolve/{token}", applyRouteMiddleware(s.resolvePublicShare, nil))
	r.Handle("/share/revoke/{token}", applyRouteMiddleware(s.revokeShare, protected))
}

func (s *CoreHandlers) registerUserRoutes(r chi.Router, protected routeMiddleware) {
	r.Handle("/user/data", applyRouteMiddleware(s.getUserData, protected))
	r.Handle("/user/bucket", applyRouteMiddleware(s.getUserBucketData, protected))
	r.Handle("/user/private/download_token", applyRouteMiddleware(s.getUserPrivateFileByName, protected))
	r.Handle("/user/account/delete", applyRouteMiddleware(s.deleteAccount, protected))
	r.Handle("/user/stats", applyRouteMiddleware(s.getUserStats, protected))
}

func (s *CoreHandlers) registerAPIKeyRoutes(r chi.Router, protected routeMiddleware) {
	r.Handle("/api_keys/{workspace_id}/create", applyRouteMiddleware(s.createWorkspaceAPIKey, protected))
	r.Handle("/api_keys/{workspace_id}/list", applyRouteMiddleware(s.listWorkspaceAPIKeys, protected))
	r.Handle("/api_keys/{workspace_id}/delete", applyRouteMiddleware(s.deleteWorkspaceAPIKey, protected))
	r.Handle("/api_keys/private/create", applyRouteMiddleware(s.createPrivateAPIKey, protected))
	r.Handle("/api_keys/private/list", applyRouteMiddleware(s.listPrivateAPIKeys, protected))
	r.Handle("/api_keys/private/delete", applyRouteMiddleware(s.deletePrivateAPIKey, protected))
}

func NewAPIServer(backendConfig config.BackendConfig, deps APIServerDependencies) *APIServer {
	return &APIServer{
		CoreHandlers:   NewCoreHandlers(backendConfig, deps.CoreHandlersDependencies),
		authProviders:  deps.AuthProviders,
		tokenEncryptor: deps.TokenEncryptor,
	}
}

func (s *APIServer) Handler() http.Handler {

	r := chi.NewRouter()
	r.Use(chimiddleware.RequestLogger(s.log))

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{s.backendConfig.FrontendEndpoint},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	})

	r.Use(c.Handler)

	s.registerAuthRoutes(r)
	s.registerFileRoutes(r, s.authMiddleware)
	s.registerWorkspaceRoutes(r, s.authMiddleware)
	s.registerSharingRoutes(r, s.authMiddleware)
	s.registerUserRoutes(r, s.authMiddleware)
	s.registerAPIKeyRoutes(r, s.authMiddleware)

	return r
}

func (s *APIServer) Start() {
	s.log.Infof("listening on %s", s.backendConfig.ListenPort)
	http.ListenAndServe("0.0.0.0"+s.backendConfig.ListenPort, s.Handler())
}

func (s *APIServer) registerAuthRoutes(r chi.Router) {
	// auth
	r.Handle("/auth/{provider}/login", http.HandlerFunc(s.oauthLoginHandler))
	r.Handle("/auth/{provider}/callback", http.HandlerFunc(s.authCallbackHandler))
	r.Handle("/auth/is_valid", http.HandlerFunc(s.isValid))
	r.Handle("/auth/logout", http.HandlerFunc(s.logout))
}
