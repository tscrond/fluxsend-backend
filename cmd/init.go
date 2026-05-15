package main

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

func InitMailSender(provider string) (mailtypes.EmailSender, error) {
	return mailfactory.NewEmailService(provider)
}

func InitObjectStorage(log *zap.SugaredLogger, backendEndpoint, storageProvider string) (storagetypes.ObjectStorage, error) {
	log.Infof("backend endpoint: %s", backendEndpoint)

	return storagefactory.NewStorageProvider(log, storageProvider)
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
	initializedProviders, err := auth.InitAuthProviders(authConfig, "google", "github")
	if err != nil {
		return nil, fmt.Errorf("error initializing auth providers: %w", err)
	}
	return initializedProviders, nil
}

func runHTTPServers(log *zap.SugaredLogger, servers ...namedHTTPServer) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	group, groupCtx := errgroup.WithContext(ctx)

	for _, item := range servers {
		item := item
		group.Go(func() error {
			log.Infof("%s listening on %s", item.name, item.srv.Addr)
			err := item.srv.ListenAndServe()
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
			if err := item.srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
		}

		return nil
	})

	return group.Wait()
}
