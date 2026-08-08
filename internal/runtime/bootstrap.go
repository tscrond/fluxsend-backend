package runtime

import (
	"fmt"

	"github.com/microcosm-cc/bluemonday"
	"github.com/tscrond/fluxsend-backend/internal/cdn"
	storagetypes "github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	"github.com/tscrond/fluxsend-backend/internal/config"
	mailtypes "github.com/tscrond/fluxsend-backend/internal/mailservice/types"
	"github.com/tscrond/fluxsend-backend/internal/repo"
	"github.com/tscrond/fluxsend-backend/internal/service"
	"go.uber.org/zap"
)

type baseRuntime struct {
	Repository             repo.Repository
	BucketHandler          storagetypes.ObjectStorage
	CloudFrontSigner       *cdn.CloudFrontURLSigner
	EmailSender            mailtypes.EmailSender
	HTMLSanitizationPolicy *bluemonday.Policy
	FileService            service.FileService
	ShareService           service.ShareService
	UserService            service.UserService
	WorkspaceService       service.WorkspaceService
	WorkspaceFileService   service.WorkspaceFileService
	ApiKeyService          service.APIKeyService
}

func (rt *baseRuntime) Close(log *zap.SugaredLogger) {
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

func BuildBaseRuntime(log *zap.SugaredLogger, baseConfig *config.BaseRuntimeConfig) (*baseRuntime, error) {

	log.Infof("backend endpoint: %s\n frontend endpoint: %s", baseConfig.BackendEndpoint, baseConfig.FrontendEndpoint)

	repository, err := InitRepository(baseConfig.DB.ConnString())
	if err != nil {
		return nil, fmt.Errorf("failed to init repository: %w", err)
	}

	storageProvider := baseConfig.Storage.Provider
	log.Infof("selected storage provider: %s", storageProvider)

	enableCloudFrontDownloads := baseConfig.CloudFront.EnableDownloads

	bucketHandler, err := InitObjectStorage(log, baseConfig.BackendEndpoint, baseConfig.Storage)
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
			baseConfig.CloudFront.Domain,
			baseConfig.CloudFront.KeyPairID,
			baseConfig.CloudFront.PrivateKeyPath,
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

	emailSender, err := InitMailSender(baseConfig.Mail.Provider, baseConfig.Mail)
	if err != nil {
		bucketHandler.Close()
		repository.Close()
		return nil, fmt.Errorf("failed to init mail sender: %w", err)
	}

	fileSvc := service.NewFileService(log, repository.Queries(), bucketHandler, htmlSanitizationPolicy, repository)
	shareSvc := service.NewShareService(log, repository.Queries(), bucketHandler, cloudFrontSigner, emailSender, baseConfig.BackendEndpoint, baseConfig.FrontendEndpoint, baseConfig.MailFrom)
	userSvc := service.NewUserService(log, repository.Queries(), bucketHandler)
	workspaceSvc := service.NewWorkspaceService(log, repository.Queries(), repository)
	workspaceFileSvc := service.NewWorkspaceFileServiceWithRepository(log, repository.Queries(), bucketHandler, repository)
	apiKeySvc := service.NewAPIKeyService(log, repository)

	return &baseRuntime{
		Repository:             repository,
		BucketHandler:          bucketHandler,
		CloudFrontSigner:       cloudFrontSigner,
		EmailSender:            emailSender,
		HTMLSanitizationPolicy: htmlSanitizationPolicy,
		FileService:            fileSvc,
		ShareService:           shareSvc,
		UserService:            userSvc,
		WorkspaceService:       workspaceSvc,
		WorkspaceFileService:   workspaceFileSvc,
		ApiKeyService:          apiKeySvc,
	}, nil
}
