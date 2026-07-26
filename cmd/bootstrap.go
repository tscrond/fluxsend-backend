package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/tscrond/fluxsend-backend/internal/api"
	"github.com/tscrond/fluxsend-backend/internal/auth"
	"github.com/tscrond/fluxsend-backend/internal/cdn"
	storagetypes "github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	"github.com/tscrond/fluxsend-backend/internal/config"
	mailtypes "github.com/tscrond/fluxsend-backend/internal/mailservice/types"
	"github.com/tscrond/fluxsend-backend/internal/repo"
	"github.com/tscrond/fluxsend-backend/internal/service"
	"github.com/tscrond/fluxsend-backend/internal/tokencrypto"
	"github.com/tscrond/fluxsend-backend/pkg"
	"go.uber.org/zap"
)

type appRuntime struct {
	Repository             repo.Repository
	BucketHandler          storagetypes.ObjectStorage
	CloudFrontSigner       *cdn.CloudFrontURLSigner
	EmailSender            mailtypes.EmailSender
	AuthProviders          map[string]auth.AuthProvider
	TokenEncryptor         *tokencrypto.Encryptor
	HTMLSanitizationPolicy *bluemonday.Policy
	FileService            service.FileService
	ShareService           service.ShareService
	UserService            service.UserService
	WorkspaceService       service.WorkspaceService
	WorkspaceFileService   service.WorkspaceFileService
	ApiKeyService          service.APIKeyService
}

func (rt *appRuntime) Close(log *zap.SugaredLogger) {
	if rt == nil {
		return
	}
	if rt.BucketHandler != nil {
		if err := rt.BucketHandler.Close(); err != nil {
			log.Warnw("failed to close bucket handler", "error", err)
		}
	}
	if rt.Repository != nil {
		if err := rt.Repository.Close(); err != nil {
			log.Warnw("failed to close repository", "error", err)
		}
	}
}

func buildRuntime(log *zap.SugaredLogger, apiConfig *config.APIServerConfig) (*appRuntime, error) {

	log.Infof("backend endpoint: %s\n frontend endpoint: %s", apiConfig.BackendEndpoint, apiConfig.FrontendEndpoint)

	repository, err := InitRepository(apiConfig.DB.ConnString())
	if err != nil {
		return nil, fmt.Errorf("failed to init repository: %w", err)
	}

	storageProvider := strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_PROVIDER")))
	if storageProvider == "" {
		// Auto-detect for chart setups where STORAGE_PROVIDER is not explicitly provided.
		if os.Getenv("AWS_REGION") != "" || os.Getenv("AWS_ACCESS_KEY_ID") != "" || os.Getenv("AWS_SECRET_ACCESS_KEY") != "" {
			storageProvider = "s3"
		} else {
			storageProvider = "gcs"
		}
	}
	log.Infof("selected storage provider: %s", storageProvider)

	enableCloudFrontDownloads, err := pkg.GetEnvBool("ENABLE_CLOUDFRONT_DOWNLOADS", false)
	if err != nil {
		repository.Close()
		return nil, fmt.Errorf("getEnvBool: %w", err)
	}

	bucketHandler, err := InitObjectStorage(log, apiConfig.BackendEndpoint, storageProvider)
	if err != nil {
		repository.Close()
		return nil, fmt.Errorf("failed to init object storage: %w", err)
	}

	var cloudFrontSigner *cdn.CloudFrontURLSigner
	if enableCloudFrontDownloads {
		if storageProvider != "s3" {
			bucketHandler.Close()
			repository.Close()
			return nil, fmt.Errorf("ENABLE_CLOUDFRONT_DOWNLOADS requires STORAGE_PROVIDER=s3")
		}

		cloudFrontSigner, err = cdn.NewCloudFrontURLSigner(
			bucketHandler.GetBucketBaseName(),
			os.Getenv("CLOUDFRONT_DOMAIN"),
			os.Getenv("CLOUDFRONT_KEY_PAIR_ID"),
			os.Getenv("CLOUDFRONT_PRIVATE_KEY_PATH"),
		)
		if err != nil {
			bucketHandler.Close()
			repository.Close()
			return nil, fmt.Errorf("failed to init CloudFront signer: %w", err)
		}

		log.Info("CloudFront download signing enabled")
	} else {
		log.Info("CloudFront download signing disabled; using storage signed URLs")
	}

	htmlSanitizationPolicy := bluemonday.UGCPolicy()

	provider := "standard"
	emailSender, err := InitMailSender(provider)
	if err != nil {
		bucketHandler.Close()
		repository.Close()
		return nil, fmt.Errorf("failed to init mail sender: %w", err)
	}

	authConfig := config.AuthConfig{
		GoogleOAuthConfig: config.GoogleOAuthConfig{
			ClientID:     apiConfig.GoogleClientID,
			ClientSecret: apiConfig.GoogleClientSecret,
			RedirectURL:  apiConfig.BackendEndpoint + "/auth/google/callback",
			Scopes:       []string{"email", "profile"},
		},
		GithubOAuthConfig: config.GithubOAuthConfig{
			ClientID:     apiConfig.GitHubClientID,
			ClientSecret: apiConfig.GitHubClientSecret,
			RedirectURL:  apiConfig.BackendEndpoint + "/auth/github/callback",
			Scopes:       []string{"user:email", "read:user"},
		},
		TokenEncryptionKey: apiConfig.TokenEncryptionKey,
	}

	authProviders, err := InitAuth(authConfig)
	if err != nil {
		bucketHandler.Close()
		repository.Close()
		return nil, fmt.Errorf("failed to init auth: %w", err)
	}

	tokenEncryptor, err := tokencrypto.New(apiConfig.TokenEncryptionKey)
	if err != nil {
		bucketHandler.Close()
		repository.Close()
		return nil, fmt.Errorf("failed to initialize token encryptor: %w", err)
	}

	fileSvc := service.NewFileService(log, repository.Queries(), bucketHandler, htmlSanitizationPolicy, repository)
	shareSvc := service.NewShareService(log, repository.Queries(), bucketHandler, cloudFrontSigner, emailSender, apiConfig.BackendEndpoint, apiConfig.FrontendEndpoint, apiConfig.MailFrom)
	userSvc := service.NewUserService(log, repository.Queries(), bucketHandler)
	workspaceSvc := service.NewWorkspaceService(log, repository.Queries(), repository)
	workspaceFileSvc := service.NewWorkspaceFileService(log, repository.Queries(), bucketHandler)
	apiKeySvc := service.NewAPIKeyService(log, repository)

	return &appRuntime{
		Repository:             repository,
		BucketHandler:          bucketHandler,
		CloudFrontSigner:       cloudFrontSigner,
		EmailSender:            emailSender,
		AuthProviders:          authProviders,
		TokenEncryptor:         tokenEncryptor,
		HTMLSanitizationPolicy: htmlSanitizationPolicy,
		FileService:            fileSvc,
		ShareService:           shareSvc,
		UserService:            userSvc,
		WorkspaceService:       workspaceSvc,
		WorkspaceFileService:   workspaceFileSvc,
		ApiKeyService:          apiKeySvc,
	}, nil
}

func buildAPIServer(log *zap.SugaredLogger, apiConfig *config.APIServerConfig, runtime *appRuntime) *api.APIServer {
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
		AuthProviders:  runtime.AuthProviders,
		TokenEncryptor: runtime.TokenEncryptor,
	})
}

func buildCLIServer(log *zap.SugaredLogger, cliConfig *config.CLIServerConfig, runtime *appRuntime) *api.CLIServer {
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
