package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/tscrond/fluxsend-backend/internal/config"
	chimiddleware "github.com/tscrond/fluxsend-backend/internal/middleware"
	"github.com/tscrond/fluxsend-backend/internal/repo"
	"github.com/tscrond/fluxsend-backend/internal/service"
	"go.uber.org/zap"
)

type CLIServer struct {
	log           *zap.SugaredLogger
	backendConfig config.BackendConfig
	routePrefix   string
	repository    repo.Repository
	files         service.FileService
	users         service.UserService
}

type CLIServerDependencies struct {
	Log        *zap.SugaredLogger
	Repository repo.Repository
	Files      service.FileService
	Users      service.UserService
}

func NewCLIServer(backendConfig config.BackendConfig, routePrefix string, deps CLIServerDependencies) *CLIServer {
	return &CLIServer{
		log:           deps.Log,
		backendConfig: backendConfig,
		routePrefix:   routePrefix,
		repository:    deps.Repository,
		files:         deps.Files,
		users:         deps.Users,
	}
}

func (s *CLIServer) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestLogger(s.log))

	r.Route(s.routePrefix, func(r chi.Router) {
		// r.Handle("/auth/login", http.HandlerFunc(s.oauthLoginHandler))
		// r.Handle("/files/upload", s.authMiddleware(http.HandlerFunc(s.uploadHandler)))
		// r.Handle("/user/data", s.authMiddleware(http.HandlerFunc(s.getUserData)))
	})

	return r
}

func (s *CLIServer) Start() {
	s.log.Infof("CLI server listening on %s", s.backendConfig.ListenPort)
	http.ListenAndServe("0.0.0.0"+s.backendConfig.ListenPort, s.Handler())

}
