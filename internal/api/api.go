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

type APIServer struct {
	log              *zap.SugaredLogger
	backendConfig    config.BackendConfig
	bucketHandler    storagetypes.ObjectStorage
	cloudFrontSigner *cdn.CloudFrontURLSigner
	emailSender      mailtypes.EmailSender
	repository       repo.Repository
	authProviders    map[string]auth.AuthProvider
	tokenEncryptor   *tokencrypto.Encryptor

	files          service.FileService
	shares         service.ShareService
	users          service.UserService
	workspaces     service.WorkspaceService
	workspaceFiles service.WorkspaceFileService
	apiKeys        service.APIKeyService
}

type APIServerDependencies struct {
	Log              *zap.SugaredLogger
	EmailSender      mailtypes.EmailSender
	BucketHandler    storagetypes.ObjectStorage
	CloudFrontSigner *cdn.CloudFrontURLSigner
	Repository       repo.Repository
	AuthProviders    map[string]auth.AuthProvider
	TokenEncryptor   *tokencrypto.Encryptor
	Files            service.FileService
	Shares           service.ShareService
	Users            service.UserService
	Workspaces       service.WorkspaceService
	WorkspaceFiles   service.WorkspaceFileService
	ApiKeys          service.APIKeyService
}

func NewAPIServer(backendConfig config.BackendConfig, deps APIServerDependencies) *APIServer {
	return &APIServer{
		log:              deps.Log,
		backendConfig:    backendConfig,
		bucketHandler:    deps.BucketHandler,
		cloudFrontSigner: deps.CloudFrontSigner,
		emailSender:      deps.EmailSender,
		repository:       deps.Repository,
		authProviders:    deps.AuthProviders,
		tokenEncryptor:   deps.TokenEncryptor,
		files:            deps.Files,
		shares:           deps.Shares,
		users:            deps.Users,
		workspaces:       deps.Workspaces,
		workspaceFiles:   deps.WorkspaceFiles,
		apiKeys:          deps.ApiKeys,
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
	s.registerFileRoutes(r)
	s.registerWorkspaceRoutes(r)
	s.registerSharingRoutes(r)
	s.registerUserRoutes(r)
	s.registerAPIKeyRoutes(r)

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

func (s *APIServer) registerFileRoutes(r chi.Router) {
	r.Handle("/files/upload", s.authMiddleware(http.HandlerFunc(s.uploadHandler)))
	r.Handle("/files/share", s.authMiddleware(http.HandlerFunc(s.shareWith)))
	r.Handle("/files/tree", s.authMiddleware(http.HandlerFunc(s.getFilesTree)))
	r.Handle("/files/move", s.authMiddleware(http.HandlerFunc(s.moveFile)))

	r.Handle("/files/received", s.authMiddleware(http.HandlerFunc(s.getDataSharedForUser)))
	r.Handle("/files/received/unseen_count", s.authMiddleware(http.HandlerFunc(s.getUnseenReceivedCount)))
	r.Handle("/files/received/mark_seen", s.authMiddleware(http.HandlerFunc(s.markReceivedSeen)))
	r.Handle("/files/shared_by_user", s.authMiddleware(http.HandlerFunc(s.getDataSharedByUser)))
	r.Handle("/files/quick_share", s.authMiddleware(http.HandlerFunc(s.quickShare)))
	r.Handle("/files/delete", s.authMiddleware(http.HandlerFunc(s.deleteFile)))
	r.Handle("/files/delete/batch", s.authMiddleware(http.HandlerFunc(s.deleteFilesBatch)))
	r.Handle("/files/{checksum}/note", s.authMiddleware(http.HandlerFunc(s.fileNotesHandler)))

	r.Handle("/folders", s.authMiddleware(http.HandlerFunc(s.foldersHandler)))
	r.Handle("/folders/move", s.authMiddleware(http.HandlerFunc(s.moveFolder)))
}

func (s *APIServer) registerWorkspaceRoutes(r chi.Router) {
	r.Handle("/workspaces/create", s.authMiddleware(http.HandlerFunc(s.createWorkspace)))
	r.Handle("/workspaces/list", s.authMiddleware(http.HandlerFunc(s.listWorkspaces)))
	r.Handle("/workspaces/members", s.authMiddleware(http.HandlerFunc(s.getWorkspaceMembers)))
	r.Handle("/workspaces/members/remove", s.authMiddleware(http.HandlerFunc(s.removeWorkspaceMember)))
	r.Handle("/workspaces/invites", s.authMiddleware(http.HandlerFunc(s.getWorkspaceInvites)))
	r.Handle("/workspaces/invites/mine", s.authMiddleware(http.HandlerFunc(s.getMyWorkspaceInvites)))
	r.Handle("/workspaces/invites/create", s.authMiddleware(http.HandlerFunc(s.createWorkspaceInvite)))
	r.Handle("/workspaces/invites/accept", s.authMiddleware(http.HandlerFunc(s.acceptWorkspaceInvite)))
	r.Handle("/workspaces/invites/reject", s.authMiddleware(http.HandlerFunc(s.rejectWorkspaceInvite)))
	r.Handle("/workspaces/invites/delete", s.authMiddleware(http.HandlerFunc(s.deleteWorkspaceInvite)))
	r.Handle("/workspaces/delete", s.authMiddleware(http.HandlerFunc(s.deleteWorkspace)))
	r.Handle("/workspaces/rename", s.authMiddleware(http.HandlerFunc(s.renameWorkspace)))
	r.Handle("/workspaces/members/role", s.authMiddleware(http.HandlerFunc(s.changeMemberRole)))

	r.Handle("/workspaces/{workspace_id}/files/tree", s.authMiddleware(http.HandlerFunc(s.getWorkspaceFilesTree)))
	r.Handle("/workspaces/{workspace_id}/files/upload", s.authMiddleware(http.HandlerFunc(s.uploadWorkspaceFile)))
	r.Handle("/workspaces/{workspace_id}/files/mkdir", s.authMiddleware(http.HandlerFunc(s.mkdirWorkspace)))
	r.Handle("/workspaces/{workspace_id}/files/delete", s.authMiddleware(http.HandlerFunc(s.deleteWorkspaceFile)))
	r.Handle("/workspaces/{workspace_id}/files/folder/delete", s.authMiddleware(http.HandlerFunc(s.deleteWorkspaceFolder)))
	r.Handle("/workspaces/{workspace_id}/files/move", s.authMiddleware(http.HandlerFunc(s.moveWorkspaceFile)))
	r.Handle("/workspaces/{workspace_id}/files/folder/move", s.authMiddleware(http.HandlerFunc(s.moveWorkspaceFolder)))
	r.Handle("/workspaces/{workspace_id}/files/download", s.authMiddleware(http.HandlerFunc(s.downloadWorkspaceFile)))
	r.Handle("/workspaces/{workspace_id}/quota", s.authMiddleware(http.HandlerFunc(s.getWorkspaceQuota)))
}

func (s *APIServer) registerSharingRoutes(r chi.Router) {
	r.Handle("/d/private/{token}", s.authMiddleware(http.HandlerFunc(s.downloadThroughProxyPersonal)))
	r.Handle("/d/{token}", http.HandlerFunc(s.downloadThroughProxy))
	r.Handle("/share/info/{token}", http.HandlerFunc(s.publicShareInfo))
	r.Handle("/share/resolve/{token}", http.HandlerFunc(s.resolvePublicShare))
	r.Handle("/share/revoke/{token}", s.authMiddleware(http.HandlerFunc(s.revokeShare)))
}

func (s *APIServer) registerUserRoutes(r chi.Router) {
	r.Handle("/user/data", s.authMiddleware(http.HandlerFunc(s.getUserData)))
	r.Handle("/user/bucket", s.authMiddleware(http.HandlerFunc(s.getUserBucketData)))
	r.Handle("/user/private/download_token", s.authMiddleware(http.HandlerFunc(s.getUserPrivateFileByName)))
	r.Handle("/user/account/delete", s.authMiddleware(http.HandlerFunc(s.deleteAccount)))
	r.Handle("/user/stats", s.authMiddleware(http.HandlerFunc(s.getUserStats)))
}

func (s *APIServer) registerAPIKeyRoutes(r chi.Router) {
	r.Handle("/api_keys/{workspace_id}/create", s.authMiddleware(http.HandlerFunc(s.createWorkspaceAPIKey)))
	r.Handle("/api_keys/{workspace_id}/list", s.authMiddleware(http.HandlerFunc(s.listWorkspaceAPIKeys)))
	r.Handle("/api_keys/{workspace_id}/delete", s.authMiddleware(http.HandlerFunc(s.deleteWorkspaceAPIKey)))
	r.Handle("/api_keys/private/create", s.authMiddleware(http.HandlerFunc(s.createPrivateAPIKey)))
	r.Handle("/api_keys/private/list", s.authMiddleware(http.HandlerFunc(s.listPrivateAPIKeys)))
	r.Handle("/api_keys/private/delete", s.authMiddleware(http.HandlerFunc(s.deletePrivateAPIKey)))
}
