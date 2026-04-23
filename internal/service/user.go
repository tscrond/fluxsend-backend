package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	storagetypes "github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	"github.com/tscrond/fluxsend-backend/internal/mappings"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"github.com/tscrond/fluxsend-backend/pkg"
)

// DeleteAccountResult carries the outcome of an account deletion operation.
type DeleteAccountResult struct {
	AccountID     string `json:"id"`
	Email         string `json:"email"`
	UserName      string `json:"user_name"`
	BucketName    string `json:"bucket_name"`
	BucketDeleted bool   `json:"bucket_deleted"`
}

// UserService encapsulates business logic for user profile and account operations.
type UserService interface {
	GetBucketData(ctx context.Context, userID uuid.UUID, userInternalID string) (*mappings.BucketData, error)
	GetPrivateDownloadToken(ctx context.Context, userID uuid.UUID, fileName string) (string, error)
	DeleteAccount(ctx context.Context, userID uuid.UUID, userInternalID, userName string, deleteStorageData bool) (*DeleteAccountResult, error)
}

type userService struct {
	queries *sqlc.Queries
	storage storagetypes.ObjectStorage
}

func NewUserService(queries *sqlc.Queries, storage storagetypes.ObjectStorage) UserService {
	return &userService{
		queries: queries,
		storage: storage,
	}
}

func (s *userService) GetBucketData(ctx context.Context, userID uuid.UUID, userInternalID string) (*mappings.BucketData, error) {
	filesByOwner, err := s.queries.GetFilesByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}

	bucketName, err := resolveUserBucketName(ctx, s.queries, s.storage.GetBucketBaseName(), userID, userInternalID)
	if err != nil {
		return nil, err
	}

	objects := make([]mappings.ObjectMedatata, 0, len(filesByOwner))
	for _, f := range filesByOwner {
		objects = append(objects, mappings.ObjectMedatata{
			Name:        f.FileName,
			ContentType: f.FileType.String,
			Created:     time.Time{},
			Deleted:     time.Time{},
			Updated:     time.Time{},
			MD5:         f.Md5Checksum,
			Size:        f.Size.Int64,
			MediaLink:   "",
			Bucket:      bucketName,
		})
	}

	return &mappings.BucketData{
		BucketName:   bucketName,
		StorageClass: "STANDARD",
		TimeCreated:  time.Time{},
		Labels:       nil,
		Objects:      objects,
	}, nil
}

func (s *userService) GetPrivateDownloadToken(ctx context.Context, userID uuid.UUID, fileName string) (string, error) {
	token, err := s.queries.GetPrivateDownloadTokenByFileName(ctx, sqlc.GetPrivateDownloadTokenByFileNameParams{
		FileName: fileName,
		OwnerID:  userID,
	})
	if err != nil {
		return "", err
	}
	return token.String, nil
}

func (s *userService) DeleteAccount(ctx context.Context, userID uuid.UUID, userInternalID, userName string, deleteStorageData bool) (*DeleteAccountResult, error) {
	bucketName := pkg.GetUserBucketName(s.storage.GetBucketBaseName(), userInternalID)

	result := &DeleteAccountResult{
		BucketName:    bucketName,
		BucketDeleted: false,
		UserName:      userName,
	}

	if deleteStorageData {
		if err := s.storage.DeleteBucket(ctx, bucketName); err != nil {
			log.Printf("failed to delete bucket %s: %v", bucketName, err)
		} else {
			result.BucketDeleted = true
		}
	}

	deletedUser, err := s.queries.DeleteAccount(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("deleting account from DB: %w", err)
	}

	result.AccountID = deletedUser.ID.String()
	result.Email = deletedUser.UserEmail
	return result, nil
}
