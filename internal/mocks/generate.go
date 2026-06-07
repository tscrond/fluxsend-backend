package mocks

//go:generate mockgen -destination=./mock_repository.go -package=mocks github.com/tscrond/fluxsend-backend/internal/repo Repository
//go:generate mockgen -destination=./mock_object_storage.go -package=mocks github.com/tscrond/fluxsend-backend/internal/cloud_storage/types ObjectStorage
//go:generate mockgen -destination=./mock_auth_provider.go -package=mocks github.com/tscrond/fluxsend-backend/internal/auth AuthProvider
//go:generate mockgen -destination=./mock_email_sender.go -package=mocks github.com/tscrond/fluxsend-backend/internal/mailservice/types EmailSender
//go:generate mockgen -source=../repo/sqlc/querier.go -destination=./mock_querier.go -package=mocks Querier
