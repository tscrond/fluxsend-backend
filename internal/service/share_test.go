package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/tscrond/fluxsend-backend/internal/mocks"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
)

func newShareTestSvc(q *mocks.MockQuerier, stor *mocks.MockObjectStorage, email *mocks.MockEmailSender) ShareService {
	return NewShareService(zap.NewNop().Sugar(), q, stor, nil, email, "https://api.example.com", "https://app.example.com", "noreply@example.com")
}

// --- GetSharedForUser -----------------------------------------------------

func TestShareService_GetSharedForUser_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	now := time.Now().Add(24 * time.Hour)
	ownerID := uuid.New()

	rows := []sqlc.GetFilesSharedWithUserRow{{
		OwnerID:      ownerID,
		FileName:     "doc.pdf",
		FileType:     sql.NullString{Valid: true, String: "application/pdf"},
		Md5Checksum:  "abc",
		SharedBy:     sql.NullString{Valid: true, String: "alice@example.com"},
		SharedFor:    sql.NullString{Valid: true, String: "bob@example.com"},
		SharingToken: "tok123",
		ExpiresAt:    now,
		Size:         sql.NullInt64{Valid: true, Int64: 1024},
		FileID:       sql.NullInt32{Valid: true, Int32: 1},
	}}
	q.EXPECT().GetFilesSharedWithUser(gomock.Any(), sql.NullString{Valid: true, String: "bob@example.com"}).
		Return(rows, nil)

	result, err := svc.GetSharedForUser(context.Background(), "bob@example.com")
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "doc.pdf", result[0].FileName)
	assert.Equal(t, "alice@example.com", result[0].SharedBy)
	assert.Equal(t, "tok123", result[0].SharingToken)
}

func TestShareService_GetSharedForUser_EmptyList(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	q.EXPECT().GetFilesSharedWithUser(gomock.Any(), gomock.Any()).Return(nil, nil)

	result, err := svc.GetSharedForUser(context.Background(), "nobody@example.com")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestShareService_GetSharedForUser_DBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	dbErr := errors.New("connection error")
	q.EXPECT().GetFilesSharedWithUser(gomock.Any(), gomock.Any()).Return(nil, dbErr)

	_, err := svc.GetSharedForUser(context.Background(), "user@example.com")
	assert.ErrorIs(t, err, dbErr)
}

// --- GetSharedByUser ------------------------------------------------------

func TestShareService_GetSharedByUser_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	ownerID := uuid.New()
	rows := []sqlc.GetFilesSharedByUserRow{{
		OwnerID:      ownerID,
		FileName:     "report.docx",
		Md5Checksum:  "def",
		SharedBy:     sql.NullString{Valid: true, String: "alice@example.com"},
		SharedFor:    sql.NullString{Valid: true, String: "bob@example.com"},
		SharingToken: "sharetok",
		ExpiresAt:    time.Now().Add(time.Hour),
	}}
	q.EXPECT().GetFilesSharedByUser(gomock.Any(), sql.NullString{Valid: true, String: "alice@example.com"}).
		Return(rows, nil)

	result, err := svc.GetSharedByUser(context.Background(), "alice@example.com")
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "report.docx", result[0].FileName)
}

// --- CountUnseen ----------------------------------------------------------

func TestShareService_CountUnseen(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	q.EXPECT().CountUnseenShares(gomock.Any(), sql.NullString{Valid: true, String: "bob@example.com"}).
		Return(int64(3), nil)

	count, err := svc.CountUnseen(context.Background(), "bob@example.com")
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

// --- MarkSeen -------------------------------------------------------------

func TestShareService_MarkSeen_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	q.EXPECT().MarkShareSeen(gomock.Any(), sqlc.MarkShareSeenParams{
		SharingToken: "tok123",
		SharedFor:    sql.NullString{Valid: true, String: "bob@example.com"},
	}).Return(sqlc.Share{}, nil)

	err := svc.MarkSeen(context.Background(), "bob@example.com", "tok123")
	assert.NoError(t, err)
}

// --- GetPublicShareInfo ---------------------------------------------------

func TestShareService_GetPublicShareInfo_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	expiresAt := time.Now().Add(24 * time.Hour)
	q.EXPECT().GetPublicShareMetadata(gomock.Any(), "valid-token").
		Return(sqlc.GetPublicShareMetadataRow{
			FileName:       "important.zip",
			ExpiresAt:      expiresAt,
			PasswordHash:   sql.NullString{Valid: false},
			FailedAttempts: 0,
		}, nil)

	info, err := svc.GetPublicShareInfo(context.Background(), "valid-token")
	require.NoError(t, err)
	assert.Equal(t, "important.zip", info.FileName)
	assert.False(t, info.PasswordProtected)
}

func TestShareService_GetPublicShareInfo_PasswordProtected(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	q.EXPECT().GetPublicShareMetadata(gomock.Any(), "pw-token").
		Return(sqlc.GetPublicShareMetadataRow{
			FileName:  "secret.zip",
			ExpiresAt: time.Now().Add(time.Hour),
			PasswordHash: sql.NullString{
				Valid:  true,
				String: "$2a$10$somethinghashed",
			},
		}, nil)

	info, err := svc.GetPublicShareInfo(context.Background(), "pw-token")
	require.NoError(t, err)
	assert.True(t, info.PasswordProtected)
}

func TestShareService_GetPublicShareInfo_Expired(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	q.EXPECT().GetPublicShareMetadata(gomock.Any(), "expired-token").
		Return(sqlc.GetPublicShareMetadataRow{
			ExpiresAt: time.Now().Add(-time.Hour), // in the past
		}, nil)

	_, err := svc.GetPublicShareInfo(context.Background(), "expired-token")
	assert.ErrorIs(t, err, ErrTokenExpired)
}

func TestShareService_GetPublicShareInfo_TooManyFailedAttempts(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	q.EXPECT().GetPublicShareMetadata(gomock.Any(), "blocked-token").
		Return(sqlc.GetPublicShareMetadataRow{
			ExpiresAt:      time.Now().Add(time.Hour),
			FailedAttempts: maxShareFailedAttempts,
		}, nil)

	_, err := svc.GetPublicShareInfo(context.Background(), "blocked-token")
	assert.ErrorIs(t, err, ErrShareBlocked)
}

func TestShareService_GetPublicShareInfo_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	q.EXPECT().GetPublicShareMetadata(gomock.Any(), "ghost-token").
		Return(sqlc.GetPublicShareMetadataRow{}, sql.ErrNoRows)

	_, err := svc.GetPublicShareInfo(context.Background(), "ghost-token")
	assert.Error(t, err)
}

// --- ResolvePublicDownload ------------------------------------------------

func TestShareService_ResolvePublicDownload_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	storageID := uuid.New()
	const bucket = "fluxsend-user"

	q.EXPECT().GetTokenExpirationTime(gomock.Any(), "tok").
		Return(time.Now().Add(time.Hour), nil)
	q.EXPECT().GetSharedFileIdFromToken(gomock.Any(), "tok").
		Return(sql.NullInt32{Valid: true, Int32: 1}, nil)
	q.EXPECT().GetBucketAndObjectFromToken(gomock.Any(), "tok").
		Return(sqlc.GetBucketAndObjectFromTokenRow{
			UserBucket:     sql.NullString{Valid: true, String: bucket},
			StorageMapping: storageID,
			FileName:       "doc.pdf",
			PasswordHash:   sql.NullString{Valid: false},
			FailedAttempts: 0,
			SharedBy:       sql.NullString{Valid: true, String: "alice@example.com"},
		}, nil)
	stor.EXPECT().GenerateSignedURL(gomock.Any(), bucket, storageID.String(), gomock.Any(), gomock.Any()).
		Return("https://signed-url.example.com/file", nil)

	result, err := svc.ResolvePublicDownload(context.Background(), "tok", "view", "")
	require.NoError(t, err)
	assert.Equal(t, "https://signed-url.example.com/file", result.URL)
	assert.Equal(t, "doc.pdf", result.FileName)
}

func TestShareService_ResolvePublicDownload_TokenExpired(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	q.EXPECT().GetTokenExpirationTime(gomock.Any(), "expired-tok").
		Return(time.Now().Add(-time.Hour), nil)

	_, err := svc.ResolvePublicDownload(context.Background(), "expired-tok", "view", "")
	assert.ErrorIs(t, err, ErrTokenExpired)
}

func TestShareService_ResolvePublicDownload_WrongPassword(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	// bcrypt hash of "correct-password"
	const hashedPw = "$2a$10$ixdWDYlTjwBqgXZU3UQ7Oe.NqpBc0sZFiNFYqWv6uNH3mB6VkRm6e"

	q.EXPECT().GetTokenExpirationTime(gomock.Any(), "pw-tok").
		Return(time.Now().Add(time.Hour), nil)
	q.EXPECT().GetSharedFileIdFromToken(gomock.Any(), "pw-tok").
		Return(sql.NullInt32{Valid: true, Int32: 1}, nil)
	q.EXPECT().GetBucketAndObjectFromToken(gomock.Any(), "pw-tok").
		Return(sqlc.GetBucketAndObjectFromTokenRow{
			PasswordHash:   sql.NullString{Valid: true, String: hashedPw},
			FailedAttempts: 0,
			SharedBy:       sql.NullString{Valid: true, String: "owner@example.com"},
		}, nil)
	// Wrong password causes increment
	q.EXPECT().IncrementShareFailedAttempts(gomock.Any(), "pw-tok").Return(int32(1), nil)

	_, err := svc.ResolvePublicDownload(context.Background(), "pw-tok", "view", "wrong-password")
	assert.ErrorIs(t, err, ErrWrongPassword)
}

func TestShareService_ResolvePublicDownload_PasswordRequired(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	q.EXPECT().GetTokenExpirationTime(gomock.Any(), "pw-tok").
		Return(time.Now().Add(time.Hour), nil)
	q.EXPECT().GetSharedFileIdFromToken(gomock.Any(), "pw-tok").
		Return(sql.NullInt32{Valid: true, Int32: 1}, nil)
	q.EXPECT().GetBucketAndObjectFromToken(gomock.Any(), "pw-tok").
		Return(sqlc.GetBucketAndObjectFromTokenRow{
			PasswordHash:   sql.NullString{Valid: true, String: "somehash"},
			FailedAttempts: 0,
		}, nil)

	_, err := svc.ResolvePublicDownload(context.Background(), "pw-tok", "view", "")
	assert.ErrorIs(t, err, ErrPasswordRequired)
}

func TestShareService_ResolvePublicDownload_Blocked(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	q.EXPECT().GetTokenExpirationTime(gomock.Any(), "blocked-tok").
		Return(time.Now().Add(time.Hour), nil)
	q.EXPECT().GetSharedFileIdFromToken(gomock.Any(), "blocked-tok").
		Return(sql.NullInt32{Valid: true, Int32: 1}, nil)
	q.EXPECT().GetBucketAndObjectFromToken(gomock.Any(), "blocked-tok").
		Return(sqlc.GetBucketAndObjectFromTokenRow{
			FailedAttempts: maxShareFailedAttempts,
		}, nil)

	_, err := svc.ResolvePublicDownload(context.Background(), "blocked-tok", "view", "")
	assert.ErrorIs(t, err, ErrShareBlocked)
}

// --- ResolvePersonalDownload ---------------------------------------------

func TestShareService_ResolvePersonalDownload_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	ownerID := uuid.New()
	storageID := uuid.New()
	const bucket = "fluxsend-user"

	q.EXPECT().GetFileIdFromToken(gomock.Any(), sql.NullString{Valid: true, String: "priv-tok"}).
		Return(int32(1), nil)
	q.EXPECT().GetBucketObjectAndOwnerFromPrivateToken(gomock.Any(), sql.NullString{Valid: true, String: "priv-tok"}).
		Return(sqlc.GetBucketObjectAndOwnerFromPrivateTokenRow{
			OwnerID:    ownerID,
			BucketName: sql.NullString{Valid: true, String: bucket},
			ObjectName: storageID,
			FileName:   "private.zip",
		}, nil)
	stor.EXPECT().GenerateSignedURL(gomock.Any(), bucket, storageID.String(), gomock.Any(), gomock.Any()).
		Return("https://signed-url.example.com/private", nil)

	result, err := svc.ResolvePersonalDownload(context.Background(), ownerID, "priv-tok", "view")
	require.NoError(t, err)
	assert.Equal(t, "private.zip", result.FileName)
}

func TestShareService_ResolvePersonalDownload_WrongOwner(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	realOwnerID := uuid.New()
	requestorID := uuid.New()

	q.EXPECT().GetFileIdFromToken(gomock.Any(), gomock.Any()).Return(int32(1), nil)
	q.EXPECT().GetBucketObjectAndOwnerFromPrivateToken(gomock.Any(), gomock.Any()).
		Return(sqlc.GetBucketObjectAndOwnerFromPrivateTokenRow{
			OwnerID: realOwnerID, // different from requestor
		}, nil)

	_, err := svc.ResolvePersonalDownload(context.Background(), requestorID, "priv-tok", "view")
	assert.ErrorIs(t, err, ErrAccessDenied)
}

// --- QuickShare -----------------------------------------------------------

func TestShareService_QuickShare_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	ownerID := uuid.New()
	fileID := int32(10)

	q.EXPECT().GetFileByOwnerAndName(gomock.Any(), sqlc.GetFileByOwnerAndNameParams{
		OwnerID:  ownerID,
		FileName: "file.txt",
	}).Return(sqlc.GetFileByOwnerAndNameRow{ID: fileID}, nil)
	q.EXPECT().InsertPublicShare(gomock.Any(), gomock.Any()).
		Return(sqlc.Share{
			SharingToken: "generated-token",
			ExpiresAt:    time.Now().Add(24 * time.Hour),
		}, nil)

	result, err := svc.QuickShare(context.Background(), "alice@example.com", ownerID, "file.txt", "24h", "")
	require.NoError(t, err)
	assert.Equal(t, "generated-token", result.SharingToken)
	assert.Contains(t, result.SharingLink, "generated-token")
}

func TestShareService_QuickShare_PasswordTooLong(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	ownerID := uuid.New()
	// Must resolve file first before password check in QuickShare
	q.EXPECT().GetFileByOwnerAndName(gomock.Any(), gomock.Any()).
		Return(sqlc.GetFileByOwnerAndNameRow{ID: 1}, nil)

	tooLongPw := string(make([]byte, maxPasswordBytes+1))
	_, err := svc.QuickShare(context.Background(), "alice@example.com", ownerID, "file.txt", "24h", tooLongPw)
	assert.ErrorIs(t, err, ErrPasswordTooLong)
}

func TestShareService_QuickShare_InvalidDuration(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	_, err := svc.QuickShare(context.Background(), "alice@example.com", uuid.New(), "file.txt", "not-a-duration", "")
	assert.Error(t, err)
}

// --- ShareWith ------------------------------------------------------------

func TestShareService_ShareWith_NoEmail(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	ownerID := uuid.New()
	fileID := int32(5)

	q.EXPECT().GetFileByOwnerAndName(gomock.Any(), gomock.Any()).
		Return(sqlc.GetFileByOwnerAndNameRow{ID: fileID, Md5Checksum: "csum"}, nil)
	q.EXPECT().InsertShare(gomock.Any(), gomock.Any()).
		Return(sqlc.Share{
			SharingToken: "share-tok",
			SharedFor:    sql.NullString{Valid: true, String: "bob@example.com"},
			SharedBy:     sql.NullString{Valid: true, String: "alice@example.com"},
			ExpiresAt:    time.Now().Add(time.Hour),
		}, nil)

	results, notifStatus, err := svc.ShareWith(
		context.Background(), "alice@example.com", ownerID, "bob@example.com",
		[]string{"doc.pdf"}, "1h", "", false,
	)
	require.NoError(t, err)
	assert.Equal(t, "not_sent", notifStatus)
	require.Len(t, results, 1)
	assert.Equal(t, "doc.pdf", results[0].File)
}

func TestShareService_ShareWith_PasswordTooLong(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	tooLongPw := string(make([]byte, maxPasswordBytes+1))
	_, _, err := svc.ShareWith(
		context.Background(), "alice@example.com", uuid.New(), "bob@example.com",
		[]string{"file.txt"}, "1h", tooLongPw, false,
	)
	assert.ErrorIs(t, err, ErrPasswordTooLong)
}

func TestShareService_ShareWith_InvalidDuration(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	_, _, err := svc.ShareWith(
		context.Background(), "alice@example.com", uuid.New(), "bob@example.com",
		[]string{"file.txt"}, "bad-duration", "", false,
	)
	assert.Error(t, err)
}

// --- RevokeShare ----------------------------------------------------------

func TestShareService_RevokeShare_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	q.EXPECT().DeleteShareByToken(gomock.Any(), sqlc.DeleteShareByTokenParams{
		SharingToken: "revoke-tok",
		SharedBy:     sql.NullString{Valid: true, String: "owner@example.com"},
	}).Return(int64(1), nil)

	err := svc.RevokeShare(context.Background(), "revoke-tok", "owner@example.com")
	assert.NoError(t, err)
}

func TestShareService_RevokeShare_NotOwner(t *testing.T) {
	ctrl := gomock.NewController(t)
	q := mocks.NewMockQuerier(ctrl)
	stor := mocks.NewMockObjectStorage(ctrl)
	emailSvc := mocks.NewMockEmailSender(ctrl)
	svc := newShareTestSvc(q, stor, emailSvc)

	q.EXPECT().DeleteShareByToken(gomock.Any(), gomock.Any()).Return(int64(0), nil)

	err := svc.RevokeShare(context.Background(), "tok", "notowner@example.com")
	assert.ErrorIs(t, err, ErrShareNotFound)
}
