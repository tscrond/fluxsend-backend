package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/tscrond/fluxsend-backend/internal/config"
	chimiddleware "github.com/tscrond/fluxsend-backend/internal/middleware"
	"github.com/tscrond/fluxsend-backend/internal/repo"
	"github.com/tscrond/fluxsend-backend/internal/service"
	"github.com/tscrond/fluxsend-backend/pkg"
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
		// r.Handle("/auth/login", s.authMiddleware(http.HandlerFunc(s.loginHandler)))
		// r.Handle("/files/tree", s.authMiddleware(http.HandlerFunc(s.getFilesTree)))
		// r.Handle("/user/data", s.authMiddleware(http.HandlerFunc(s.getUserData)))
		r.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pkg.WriteJSONResponse(w, http.StatusOK, "fluxsend developer api", nil)
		}))
		// CLI routes will be mounted here as the transport surface stabilizes.
	})

	return r
}

func (s *CLIServer) Start() {
	s.log.Infof("CLI server listening on %s", s.backendConfig.ListenPort)
	http.ListenAndServe("0.0.0.0"+s.backendConfig.ListenPort, s.Handler())

}
