package apimocks

//go:generate mockgen -destination=./mock_services.go -package=apimocks github.com/tscrond/fluxsend-backend/internal/service FileService,ShareService,WorkspaceService,UserService,WorkspaceFileService,APIKeyService
