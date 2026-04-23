package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"github.com/microcosm-cc/bluemonday"

	"github.com/tscrond/fluxsend-backend/internal/api"
	"github.com/tscrond/fluxsend-backend/internal/auth"
	"github.com/tscrond/fluxsend-backend/internal/cdn"
	storagefactory "github.com/tscrond/fluxsend-backend/internal/cloud_storage/factory"
	storagetypes "github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	"github.com/tscrond/fluxsend-backend/internal/config"
	mailfactory "github.com/tscrond/fluxsend-backend/internal/mailservice/factory"
	mailtypes "github.com/tscrond/fluxsend-backend/internal/mailservice/types"
	"github.com/tscrond/fluxsend-backend/internal/repo"
	"github.com/tscrond/fluxsend-backend/internal/service"
	"github.com/tscrond/fluxsend-backend/internal/tokencrypto"
)

func main() {
	listenPort := os.Getenv("FLUXSEND_LISTEN_PORT")
	if listenPort == "" {
		listenPort = "3000"
	}
	clientId := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	githubClientId := os.Getenv("GITHUB_OAUTH_CLIENT_ID")
	githubClientSecret := os.Getenv("GITHUB_OAUTH_CLIENT_SECRET")
	tokenEncryptionKey := os.Getenv("TOKEN_ENCRYPTION_KEY")
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

	enableCloudFrontDownloads, err := getEnvBool("ENABLE_CLOUDFRONT_DOWNLOADS", false)
	if err != nil {
		log.Fatalln(err)
	}

	bucketHandler, err := InitObjectStorage(backendEndpoint, storageProvider, repository)
	if err != nil {
		log.Fatalln(err)
	}
	defer bucketHandler.Close()

	var cloudFrontSigner *cdn.CloudFrontURLSigner
	if enableCloudFrontDownloads {
		if storageProvider != "s3" {
			log.Fatalln("ENABLE_CLOUDFRONT_DOWNLOADS requires STORAGE_PROVIDER=s3")
		}

		cloudFrontSigner, err = cdn.NewCloudFrontURLSigner(
			bucketHandler.GetBucketBaseName(),
			os.Getenv("CLOUDFRONT_DOMAIN"),
			os.Getenv("CLOUDFRONT_KEY_PAIR_ID"),
			os.Getenv("CLOUDFRONT_PRIVATE_KEY_PATH"),
		)
		if err != nil {
			log.Fatalln(err)
		}

		log.Println("CloudFront download signing enabled")
	} else {
		log.Println("CloudFront download signing disabled; using storage signed URLs")
	}

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

	authConfig := config.AuthConfig{
		GoogleOAuthConfig: config.GoogleOAuthConfig{
			ClientID:     clientId,
			ClientSecret: clientSecret,
			RedirectURL:  fmt.Sprintf("%s/auth/google/callback", backendEndpoint),
			Scopes:       []string{"email", "profile"},
		},
		GithubOAuthConfig: config.GithubOAuthConfig{
			ClientID:     githubClientId,
			ClientSecret: githubClientSecret,
			RedirectURL:  fmt.Sprintf("%s/auth/github/callback", backendEndpoint),
			Scopes:       []string{"user:email", "read:user"},
		},
		TokenEncryptionKey: tokenEncryptionKey,
	}

	authProviders, err := InitAuth(authConfig)
	if err != nil {
		log.Fatalln(err)
	}

	tokenEncryptor, err := tokencrypto.New(tokenEncryptionKey)
	if err != nil {
		log.Fatalf("failed to initialize token encryptor: %v (set TOKEN_ENCRYPTION_KEY env var)", err)
	}

	fileSvc := service.NewFileService(repository.Queries(), bucketHandler, htmlSanitizationPolicy)
	shareSvc := service.NewShareService(repository.Queries(), bucketHandler, cloudFrontSigner, emailSender, backendEndpoint, mailFrom)
	userSvc := service.NewUserService(repository.Queries(), bucketHandler)

	s := api.NewAPIServer(backendConfig, emailSender, bucketHandler, cloudFrontSigner, repository, authProviders, tokenEncryptor, fileSvc, shareSvc, userSvc)

	s.Start()
}

func InitMailSender(provider string, repository *repo.PostgresRepository) (mailtypes.EmailSender, error) {
	return mailfactory.NewEmailService(provider, repository)
}

func InitObjectStorage(backendEndpoint, storageProvider string, repository *repo.PostgresRepository) (storagetypes.ObjectStorage, error) {

	log.Printf("%s", fmt.Sprintf("%s/auth/callback", backendEndpoint))

	return storagefactory.NewStorageProvider(storageProvider, repository)
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

func getEnvBool(name string, defaultValue bool) (bool, error) {
	rawValue := strings.TrimSpace(os.Getenv(name))
	if rawValue == "" {
		return defaultValue, nil
	}

	parsedValue, err := strconv.ParseBool(rawValue)
	if err != nil {
		return false, fmt.Errorf("invalid %s value %q: %w", name, rawValue, err)
	}

	return parsedValue, nil
}

func InitAuth(authConfig config.AuthConfig) (map[string]auth.AuthProvider, error) {
	initializedProviders, err := auth.InitAuthProviders(authConfig, "google", "github")
	if err != nil {
		return nil, fmt.Errorf("error initializing auth providers: %w", err)
	}
	return initializedProviders, nil
}
