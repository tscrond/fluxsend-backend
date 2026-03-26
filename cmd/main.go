package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"github.com/microcosm-cc/bluemonday"

	"github.com/tscrond/dropper/internal/api"
	storagefactory "github.com/tscrond/dropper/internal/cloud_storage/factory"
	storagetypes "github.com/tscrond/dropper/internal/cloud_storage/types"
	"github.com/tscrond/dropper/internal/config"
	mailfactory "github.com/tscrond/dropper/internal/mailservice/factory"
	mailtypes "github.com/tscrond/dropper/internal/mailservice/types"
	"github.com/tscrond/dropper/internal/repo"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func main() {
	listenPort := os.Getenv("FLUXSEND_LISTEN_PORT")
	if listenPort == "" {
		listenPort = "3000"
	}
	clientId := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	frontendEndpoint := os.Getenv("FRONTEND_ENDPOINT")
	backendEndpoint := os.Getenv("BACKEND_ENDPOINT")
	mailFrom := os.Getenv("MAIL_FROM")
	if mailFrom == "" {
		mailFrom = "noreply@fluxsend.com"
	}

	dbHost := os.Getenv("DB_HOST")
	dbUser := os.Getenv("POSTGRES_USER")
	dbPassword := os.Getenv("POSTGRES_PASSWORD")
	dbName := os.Getenv("POSTGRES_DB")

	//postgres://<user>:<pass>@<dbhost>:5432/<dbname>?sslmode=disable
	connStr := fmt.Sprintf("postgres://%s:%s@%s:5432/%s?sslmode=disable", dbUser, dbPassword, dbHost, dbName)

	// log.Printf("db connection string: %s", connStr)
	log.Printf("backend endpoint: %s\n frontend endpoint: %s", backendEndpoint, frontendEndpoint)

	repository, err := InitRepository(connStr)
	if err != nil {
		log.Fatalln(err)
	}
	defer repository.Close()

	storageProvider := strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_PROVIDER")))
	if storageProvider == "" {
		// Auto-detect for chart setups where STORAGE_PROVIDER is not explicitly provided.
		if os.Getenv("AWS_REGION") != "" || os.Getenv("AWS_ACCESS_KEY_ID") != "" || os.Getenv("AWS_SECRET_ACCESS_KEY") != "" {
			storageProvider = "s3"
		} else {
			storageProvider = "gcs"
		}
	}
	log.Printf("selected storage provider: %s", storageProvider)

	bucketHandler, err := InitObjectStorage(backendEndpoint, storageProvider, repository)
	if err != nil {
		log.Fatalln(err)
	}
	defer bucketHandler.Close()

	htmlSanitizationPolicy := bluemonday.UGCPolicy()

	backendConfig := config.BackendConfig{
		ListenPort:             fmt.Sprintf(":%s", listenPort),
		BackendEndpoint:        backendEndpoint,
		FrontendEndpoint:       frontendEndpoint,
		MailFrom:               mailFrom,
		HTMLSanitizationPolicy: htmlSanitizationPolicy,
	}

	provider := "standard"
	emailSender, err := InitMailSender(provider, repository)
	if err != nil {
		log.Fatalln(err)
	}

	s := api.NewAPIServer(backendConfig, emailSender, bucketHandler, repository, &oauth2.Config{
		ClientID:     clientId,
		ClientSecret: clientSecret,
		RedirectURL:  fmt.Sprintf("%s/auth/callback", backendEndpoint),
		Scopes:       []string{"email", "profile"},
		Endpoint:     google.Endpoint,
	})

	s.Start()
}

func InitMailSender(provider string, repository *repo.Repository) (mailtypes.EmailSender, error) {
	return mailfactory.NewEmailService(provider, repository)
}

func InitObjectStorage(backendEndpoint, storageProvider string, repository *repo.Repository) (storagetypes.ObjectStorage, error) {

	log.Printf("%s", fmt.Sprintf("%s/auth/callback", backendEndpoint))

	return storagefactory.NewStorageProvider(storageProvider, repository)
}

func InitRepository(connString string) (*repo.Repository, error) {
	if connString == "" {
		panic("no conn string provided")
	}

	// log.Println("conn str:", connString)

	db, err := sql.Open("postgres", connString)
	if err != nil {
		return nil, err
	}

	return repo.NewRepository(db)
}
