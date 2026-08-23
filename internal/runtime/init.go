package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tscrond/fluxsend-backend/internal/auth"
	"github.com/tscrond/fluxsend-backend/internal/config"
	"github.com/tscrond/fluxsend-backend/internal/repo"
	"golang.org/x/sync/errgroup"

	storagefactory "github.com/tscrond/fluxsend-backend/internal/cloud_storage/factory"
	storagetypes "github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	mailfactory "github.com/tscrond/fluxsend-backend/internal/mailservice/factory"
	mailtypes "github.com/tscrond/fluxsend-backend/internal/mailservice/types"
	"go.uber.org/zap"
)

func InitMailSender(provider string, mailConfig config.MailConfig) (mailtypes.EmailSender, error) {
	return mailfactory.NewEmailService(provider, mailfactory.ProviderConfig{
		AWSRegion:          mailConfig.AWSRegion,
		AWSAccessKeyID:     mailConfig.AWSAccessKeyID,
		AWSSecretAccessKey: mailConfig.AWSSecretAccessKey,
		SMTPHost:           mailConfig.SMTPHost,
		SMTPPort:           mailConfig.SMTPPort,
		SMTPUsername:       mailConfig.SMTPUsername,
		SMTPPassword:       mailConfig.SMTPPassword,
	})
}

func InitObjectStorage(log *zap.SugaredLogger, backendEndpoint string, storageConfig config.StorageConfig) (storagetypes.ObjectStorage, error) {
	log.Infof("backend endpoint: %s", backendEndpoint)

	return storagefactory.NewStorageProvider(log, storageConfig.Provider, storagefactory.ProviderConfig{
		GCSBucketName:                storageConfig.GCSBucketName,
		GoogleApplicationCredentials: storageConfig.GoogleApplicationCredentials,
		GoogleProjectID:              storageConfig.GoogleProjectID,
		S3BucketName:                 storageConfig.S3BucketName,
		AWSRegion:                    storageConfig.AWSRegion,
		MinioBucketName:              storageConfig.MinioBucketName,
		MinioEndpoint:                storageConfig.MinioEndpoint,
		MinioAccessKey:               storageConfig.MinioAccessKey,
		MinioSecretKey:               storageConfig.MinioSecretKey,
		MinioUseSSL:                  storageConfig.MinioUseSSL,
	})
}

func InitRepository(connString string) (*repo.PostgresRepository, error) {
	if connString == "" {
		panic("no conn string provided")
	}

	// log.Println("conn str:", connString)

	db, err := sql.Open("postgres", connString)
	if err != nil {
		return nil, err
	}

	return repo.NewPostgresRepository(db)
}

func InitAuth(authConfig config.AuthConfig) (map[string]auth.AuthProvider, error) {
	var authProviderStrings []string
	if authConfig.GoogleOAuthConfig != nil {
		authProviderStrings = append(authProviderStrings, "google")
	}
	if authConfig.GithubOAuthConfig != nil {
		authProviderStrings = append(authProviderStrings, "github")
	}
	if authConfig.EnablePasswordAuth {
		authProviderStrings = append(authProviderStrings, "password")
	}
	if len(authProviderStrings) == 0 {
		return nil, fmt.Errorf("no authentication providers configured")
	}
	initializedProviders, err := auth.InitAuthProviders(authConfig, authProviderStrings...)
	if err != nil {
		return nil, fmt.Errorf("error initializing auth providers: %w", err)
	}
	return initializedProviders, nil
}

func RunHTTPServers(log *zap.SugaredLogger, servers ...NamedHTTPServer) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	group, groupCtx := errgroup.WithContext(ctx)

	for _, item := range servers {
		item := item
		group.Go(func() error {
			log.Infof("%s listening on %s", item.Name, item.Srv.Addr)
			err := item.Srv.ListenAndServe()
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		})
	}

	group.Go(func() error {
		<-groupCtx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		for _, item := range servers {
			if err := item.Srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
		}

		return nil
	})

	return group.Wait()
}
