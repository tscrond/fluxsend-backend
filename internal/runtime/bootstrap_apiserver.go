package runtime

import (
	"fmt"

	"github.com/tscrond/fluxsend-backend/internal/api"
	"github.com/tscrond/fluxsend-backend/internal/auth"
	"github.com/tscrond/fluxsend-backend/internal/config"
	"github.com/tscrond/fluxsend-backend/internal/tokencrypto"
	"go.uber.org/zap"
)

type apiServerRuntime struct {
	AuthProviders      map[string]auth.AuthProvider
	TokenEncryptor     *tokencrypto.Encryptor
	EnableGoogleAuth   bool
	EnableGitHubAuth   bool
	EnablePasswordAuth bool
}

func BuildAPIServerRuntime(log *zap.SugaredLogger, apiConfig *config.APIServerConfig, runtime *baseRuntime) (*apiServerRuntime, error) {
	var googleOauthConfig *config.GoogleOAuthConfig
	var githubOauthConfig *config.GithubOAuthConfig

	if apiConfig.EnableGoogleAuth {
		log.Infof("Google OAuth enabled. Client ID: %s", apiConfig.GoogleClientID)
		googleOauthConfig = &config.GoogleOAuthConfig{
			ClientID:     apiConfig.GoogleClientID,
			ClientSecret: apiConfig.GoogleClientSecret,
			RedirectURL:  apiConfig.BackendEndpoint + "/auth/google/callback",
			Scopes:       []string{"email", "profile"},
		}
	}
	if apiConfig.EnableGitHubAuth {
		log.Infof("GitHub OAuth enabled. Client ID: %s", apiConfig.GitHubClientID)
		githubOauthConfig = &config.GithubOAuthConfig{
			ClientID:     apiConfig.GitHubClientID,
			ClientSecret: apiConfig.GitHubClientSecret,
			RedirectURL:  apiConfig.BackendEndpoint + "/auth/github/callback",
			Scopes:       []string{"user:email", "read:user"},
		}
	}

	authConfig := config.AuthConfig{
		EnableGoogleAuth:   apiConfig.EnableGoogleAuth,
		EnableGithubAuth:   apiConfig.EnableGitHubAuth,
		EnablePasswordAuth: apiConfig.EnablePasswordAuth,
		GoogleOAuthConfig:  googleOauthConfig,
		GithubOAuthConfig:  githubOauthConfig,
		TokenEncryptionKey: apiConfig.TokenEncryptionKey,
	}

	if googleOauthConfig == nil && githubOauthConfig == nil && !apiConfig.EnablePasswordAuth {
		return nil, fmt.Errorf("no authentication methods enabled. Please enable at least one of Google OAuth, GitHub OAuth, or password authentication")
	}

	authProviders, err := InitAuth(authConfig)
	if err != nil {
		runtime.BucketHandler.Close()
		runtime.Repository.Close()
		return nil, fmt.Errorf("failed to init auth: %w", err)
	}
	log.Infow("initialized auth providers", "providers_count", len(authProviders), "google", authProviders["google"] != nil, "github", authProviders["github"] != nil, "password", authProviders["password"] != nil)

	tokenEncryptor, err := tokencrypto.New(apiConfig.TokenEncryptionKey)
	if err != nil {
		runtime.BucketHandler.Close()
		runtime.Repository.Close()
		return nil, fmt.Errorf("failed to initialize token encryptor: %w", err)
	}

	return &apiServerRuntime{
		AuthProviders:      authProviders,
		TokenEncryptor:     tokenEncryptor,
		EnableGoogleAuth:   apiConfig.EnableGoogleAuth,
		EnableGitHubAuth:   apiConfig.EnableGitHubAuth,
		EnablePasswordAuth: apiConfig.EnablePasswordAuth,
	}, nil
}

func BuildAPIServer(log *zap.SugaredLogger, apiConfig *config.APIServerConfig, runtime *baseRuntime, apiRuntime *apiServerRuntime) *api.APIServer {
	backendConfig := config.BackendConfig{
		ListenPort:             ":" + apiConfig.ListenPort,
		BackendEndpoint:        apiConfig.BackendEndpoint,
		FrontendEndpoint:       apiConfig.FrontendEndpoint,
		MailFrom:               apiConfig.MailFrom,
		HTMLSanitizationPolicy: runtime.HTMLSanitizationPolicy,
	}

	return api.NewAPIServer(backendConfig, api.APIServerDependencies{
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
		AuthProviders:  apiRuntime.AuthProviders,
		TokenEncryptor: apiRuntime.TokenEncryptor,
	})
}
