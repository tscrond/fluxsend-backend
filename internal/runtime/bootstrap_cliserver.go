package runtime

import (
	"github.com/tscrond/fluxsend-backend/internal/api"
	"github.com/tscrond/fluxsend-backend/internal/config"
	"go.uber.org/zap"
)

func BuildCLIServer(log *zap.SugaredLogger, cliConfig *config.CLIServerConfig, runtime *baseRuntime) *api.CLIServer {
	backendConfig := config.BackendConfig{
		ListenPort:      ":" + cliConfig.ListenPort,
		BackendEndpoint: cliConfig.BackendEndpoint,
	}

	return api.NewCLIServer(backendConfig, cliConfig.RoutePrefix, api.CLIServerDependencies{
		CoreHandlersDependencies: api.CoreHandlersDependencies{
			Log:              log,
			EmailSender:      runtime.EmailSender,
			BucketHandler:    runtime.BucketHandler,
			CloudFrontSigner: runtime.CloudFrontSigner,
			Repository:       runtime.Repository,
			Files:            runtime.FileService,
			Shares:           runtime.ShareService,
			Users:            runtime.UserService,
			Workspaces:       runtime.WorkspaceService,
			WorkspaceFiles:   runtime.WorkspaceFileService,
			ApiKeys:          runtime.ApiKeyService,
		},
	})
}
