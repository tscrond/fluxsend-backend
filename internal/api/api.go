package api

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/cors"
	storagetypes "github.com/tscrond/dropper/internal/cloud_storage/types"
	"github.com/tscrond/dropper/internal/config"
	mailtypes "github.com/tscrond/dropper/internal/mailservice/types"
	"github.com/tscrond/dropper/internal/repo"
	"golang.org/x/oauth2"
)

type APIServer struct {
	backendConfig config.BackendConfig
	bucketHandler storagetypes.ObjectStorage
	emailSender   mailtypes.EmailSender
	repository    *repo.Repository
	OAuthConfig   *oauth2.Config
}

func NewAPIServer(backendConfig config.BackendConfig, es mailtypes.EmailSender, bh storagetypes.ObjectStorage, repository *repo.Repository, oauth2conf *oauth2.Config) *APIServer {
	return &APIServer{
		backendConfig: backendConfig,
		bucketHandler: bh,
		emailSender:   es,
		repository:    repository,
		OAuthConfig:   oauth2conf,
	}
}

func (s *APIServer) Start() {

	r := chi.NewRouter()
	r.Use(middleware.Logger)

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{s.backendConfig.FrontendEndpoint},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	})

	r.Use(c.Handler)

	// auth
	r.Handle("/auth/callback", http.HandlerFunc(s.authCallback))
	r.Handle("/auth/oauth", http.HandlerFunc(s.oauthHandler))
	r.Handle("/auth/is_valid", http.HandlerFunc(s.isValid))
	r.Handle("/auth/logout", http.HandlerFunc(s.logout))

	// functionality
	r.Handle("/files/upload", s.authMiddleware(http.HandlerFunc(s.uploadHandler)))
	r.Handle("/files/share", s.authMiddleware(http.HandlerFunc(s.shareWith)))
	r.Handle("/files/tree", s.authMiddleware(http.HandlerFunc(s.getFilesTree)))
	r.Handle("/files/move", s.authMiddleware(http.HandlerFunc(s.moveFile)))

	r.Handle("/files/received", s.authMiddleware(http.HandlerFunc(s.getDataSharedForUser)))
	r.Handle("/files/shared_by_user", s.authMiddleware(http.HandlerFunc(s.getDataSharedByUser)))
	r.Handle("/files/delete", s.authMiddleware(http.HandlerFunc(s.deleteFile)))
	r.Handle("/files/{checksum}/note", s.authMiddleware(http.HandlerFunc(s.fileNotesHandler)))

	r.Handle("/folders", s.authMiddleware(http.HandlerFunc(s.foldersHandler)))
	r.Handle("/folders/move", s.authMiddleware(http.HandlerFunc(s.moveFolder)))

	r.Handle("/d/private/{token}", s.authMiddleware(http.HandlerFunc(s.downloadThroughProxyPersonal)))
	r.Handle("/d/{token}", http.HandlerFunc(s.downloadThroughProxy))

	r.Handle("/user/data", s.authMiddleware(http.HandlerFunc(s.getUserData)))
	r.Handle("/user/bucket", s.authMiddleware(http.HandlerFunc(s.getUserBucketData)))
	r.Handle("/user/private/download_token", s.authMiddleware(http.HandlerFunc(s.getUserPrivateFileByName)))
	r.Handle("/user/account/delete", s.authMiddleware(http.HandlerFunc(s.deleteAccount)))

	log.Printf("Listening on %s\n", s.backendConfig.ListenPort)
	http.ListenAndServe("0.0.0.0"+s.backendConfig.ListenPort, r)
}
