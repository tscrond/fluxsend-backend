package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"mime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/cdn"
	storagetypes "github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	templates "github.com/tscrond/fluxsend-backend/internal/mailservice/templates"
	mailtypes "github.com/tscrond/fluxsend-backend/internal/mailservice/types"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"github.com/tscrond/fluxsend-backend/pkg"
	"golang.org/x/crypto/bcrypt"
)

// ShareResult describes one share entry created by ShareWith.
type ShareResult struct {
	File         string    `json:"file"`
	SharedFor    string    `json:"shared_for"`
	SharedBy     string    `json:"shared_by"`
	Checksum     string    `json:"checksum"`
	ExpiresAt    time.Time `json:"expires_at"`
	SharingToken string    `json:"sharing_token"`
	SharingLink  string    `json:"sharing_link"`
}

// QuickShareResult describes the public share link created by QuickShare.
type QuickShareResult struct {
	SharingToken string    `json:"sharing_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	SharingLink  string    `json:"sharing_link"`
}

// SharedFileInfo is a file shared with or by a user, flattened for API responses.
type SharedFileInfo struct {
	FileID            int32     `json:"file_id"`
	OwnerID           string    `json:"owner_id"`
	FileName          string    `json:"file_name"`
	FileType          string    `json:"file_type"`
	MD5Checksum       string    `json:"md5_checksum"`
	SharedBy          string    `json:"shared_by"`
	SharedFor         string    `json:"shared_for"`
	SharingToken      string    `json:"sharing_token"`
	ExpiresAt         time.Time `json:"expires_at"`
	Size              int64     `json:"size"`
	Seen              bool      `json:"seen,omitempty"`
	PasswordProtected bool      `json:"password_protected"`
}

// ShareInfo is the public metadata about a share link, returned before authentication.
type ShareInfo struct {
	FileName          string    `json:"file_name"`
	ExpiresAt         time.Time `json:"expires_at"`
	PasswordProtected bool      `json:"password_protected"`
}

const maxShareFailedAttempts = 5

// DownloadResult carries everything the handler needs to respond to a download request.
type DownloadResult struct {
	URL      string
	FileName string
	UseProxy bool // true when CloudFront is the signer and mode is "download"
}

// ShareService encapsulates all business logic for file sharing and downloads.
type ShareService interface {
	ShareWith(ctx context.Context, sharedByEmail string, ownerID uuid.UUID, forUser string, objects []string, duration, password string, sendEmail bool) (shares []ShareResult, notificationStatus string, err error)
	QuickShare(ctx context.Context, sharedByEmail string, ownerID uuid.UUID, object, duration, password string) (*QuickShareResult, error)
	GetSharedForUser(ctx context.Context, email string) ([]SharedFileInfo, error)
	GetSharedByUser(ctx context.Context, email string) ([]SharedFileInfo, error)
	CountUnseen(ctx context.Context, email string) (int64, error)
	MarkSeen(ctx context.Context, email, token string) error
	GetPublicShareInfo(ctx context.Context, token string) (*ShareInfo, error)
	ResolvePublicDownload(ctx context.Context, token, mode, password string) (*DownloadResult, error)
	ResolvePersonalDownload(ctx context.Context, ownerID uuid.UUID, token, mode string) (*DownloadResult, error)
	RevokeShare(ctx context.Context, token, ownerEmail string) error
}

type shareService struct {
	queries          *sqlc.Queries
	storage          storagetypes.ObjectStorage
	cloudFrontSigner *cdn.CloudFrontURLSigner
	emailSender      mailtypes.EmailSender
	backendEndpoint  string
	frontendEndpoint string
	mailFrom         string
}

func NewShareService(
	queries *sqlc.Queries,
	storage storagetypes.ObjectStorage,
	cloudFrontSigner *cdn.CloudFrontURLSigner,
	emailSender mailtypes.EmailSender,
	backendEndpoint, frontendEndpoint, mailFrom string,
) ShareService {
	if mailFrom == "" {
		mailFrom = "noreply@fluxsend.com"
	}
	return &shareService{
		queries:          queries,
		storage:          storage,
		cloudFrontSigner: cloudFrontSigner,
		emailSender:      emailSender,
		backendEndpoint:  backendEndpoint,
		frontendEndpoint: frontendEndpoint,
		mailFrom:         mailFrom,
	}
}

func (s *shareService) ShareWith(ctx context.Context, sharedByEmail string, ownerID uuid.UUID, forUser string, objects []string, duration, password string, sendEmail bool) ([]ShareResult, string, error) {
	expiryDuration, err := pkg.CustomParseDuration(duration)
	if err != nil {
		return nil, "", fmt.Errorf("invalid duration %q: %w", duration, err)
	}
	expiresAt := time.Now().Add(expiryDuration)

	var hashedPassword string
	if password != "" {
		if len([]byte(password)) > maxPasswordBytes {
			return nil, "", ErrPasswordTooLong
		}
		hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, "", fmt.Errorf("hashing password: %w", err)
		}
		hashedPassword = string(hashed)
	}

	results := make([]ShareResult, 0, len(objects))
	filesForMail := make([]mailtypes.FileInfo, 0, len(objects))

	for _, objectName := range objects {
		fileData, err := s.queries.GetFileByOwnerAndName(ctx, sqlc.GetFileByOwnerAndNameParams{
			OwnerID:  ownerID,
			FileName: objectName,
		})
		if err != nil {
			log.Printf("error getting object data for %q: %v", objectName, err)
			continue
		}

		token, err := pkg.RandToken(32)
		if err != nil {
			log.Printf("error generating token for %q: %v", objectName, err)
			continue
		}

		var share sqlc.Share
		if hashedPassword != "" {
			share, err = s.queries.InsertShareWithPassword(ctx, sqlc.InsertShareWithPasswordParams{
				SharedBy:     sql.NullString{Valid: true, String: sharedByEmail},
				SharedFor:    sql.NullString{Valid: true, String: forUser},
				FileID:       sql.NullInt32{Valid: true, Int32: fileData.ID},
				ExpiresAt:    expiresAt,
				SharingToken: token,
				PasswordHash: sql.NullString{Valid: true, String: hashedPassword},
			})
		} else {
			share, err = s.queries.InsertShare(ctx, sqlc.InsertShareParams{
				SharedBy:     sql.NullString{Valid: true, String: sharedByEmail},
				SharedFor:    sql.NullString{Valid: true, String: forUser},
				FileID:       sql.NullInt32{Valid: true, Int32: fileData.ID},
				ExpiresAt:    expiresAt,
				SharingToken: token,
			})
		}
		if err != nil {
			return nil, "", fmt.Errorf("inserting share for %q: %w", objectName, err)
		}

		link := fmt.Sprintf("%s/share/%s", s.shareBaseURL(), share.SharingToken)
		results = append(results, ShareResult{
			File:         objectName,
			SharedFor:    share.SharedFor.String,
			SharedBy:     share.SharedBy.String,
			Checksum:     fileData.Md5Checksum,
			ExpiresAt:    share.ExpiresAt,
			SharingToken: share.SharingToken,
			SharingLink:  link,
		})
		filesForMail = append(filesForMail, mailtypes.FileInfo{
			FileName:    objectName,
			DownloadURL: fmt.Sprintf("%s/share/%s", s.shareBaseURL(), share.SharingToken),
		})
	}

	if !sendEmail {
		return results, "not_sent", nil
	}

	notifyErr := s.sendSharingNotification(sharedByEmail, forUser, expiresAt.Format("2006-01-02 15:04"), filesForMail)
	if notifyErr != nil {
		log.Println("issues sending email notification:", notifyErr)
		return results, "failed", nil
	}
	return results, "sent", nil
}

func (s *shareService) QuickShare(ctx context.Context, sharedByEmail string, ownerID uuid.UUID, object, duration, password string) (*QuickShareResult, error) {
	if duration == "" {
		duration = "24h"
	}
	expiryDuration, err := pkg.CustomParseDuration(duration)
	if err != nil {
		return nil, fmt.Errorf("invalid duration %q: %w", duration, err)
	}

	fileData, err := s.queries.GetFileByOwnerAndName(ctx, sqlc.GetFileByOwnerAndNameParams{
		OwnerID:  ownerID,
		FileName: object,
	})
	if err != nil {
		return nil, err
	}

	token, err := pkg.RandToken(32)
	if err != nil {
		return nil, fmt.Errorf("generating token: %w", err)
	}
	expiresAt := time.Now().Add(expiryDuration)

	var share sqlc.Share
	if password != "" {
		if len([]byte(password)) > maxPasswordBytes {
			return nil, ErrPasswordTooLong
		}
		hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hashing password: %w", err)
		}
		share, err = s.queries.InsertPublicSharePasswordProtected(ctx, sqlc.InsertPublicSharePasswordProtectedParams{
			SharedBy:     sql.NullString{Valid: true, String: sharedByEmail},
			FileID:       sql.NullInt32{Valid: true, Int32: fileData.ID},
			ExpiresAt:    expiresAt,
			SharingToken: token,
			PasswordHash: sql.NullString{Valid: true, String: string(hashed)},
		})
		if err != nil {
			return nil, fmt.Errorf("inserting password-protected public share: %w", err)
		}
	} else {
		share, err = s.queries.InsertPublicShare(ctx, sqlc.InsertPublicShareParams{
			SharedBy:     sql.NullString{Valid: true, String: sharedByEmail},
			FileID:       sql.NullInt32{Valid: true, Int32: fileData.ID},
			ExpiresAt:    expiresAt,
			SharingToken: token,
		})
		if err != nil {
			return nil, fmt.Errorf("inserting public share: %w", err)
		}
	}

	return &QuickShareResult{
		SharingToken: share.SharingToken,
		ExpiresAt:    share.ExpiresAt,
		SharingLink:  fmt.Sprintf("%s/share/%s", s.shareBaseURL(), share.SharingToken),
	}, nil
}

func (s *shareService) GetSharedForUser(ctx context.Context, email string) ([]SharedFileInfo, error) {
	rows, err := s.queries.GetFilesSharedWithUser(ctx, sql.NullString{Valid: true, String: email})
	if err != nil {
		return nil, err
	}
	result := make([]SharedFileInfo, 0, len(rows))
	for _, r := range rows {
		result = append(result, SharedFileInfo{
			FileID:            r.FileID.Int32,
			OwnerID:           r.OwnerID.String(),
			FileName:          r.FileName,
			FileType:          r.FileType.String,
			MD5Checksum:       r.Md5Checksum,
			SharedBy:          r.SharedBy.String,
			SharedFor:         r.SharedFor.String,
			SharingToken:      r.SharingToken,
			ExpiresAt:         r.ExpiresAt,
			Size:              r.Size.Int64,
			Seen:              r.ReceivedSeenAt.Valid,
			PasswordProtected: r.PasswordHash.Valid && r.PasswordHash.String != "",
		})
	}
	return result, nil
}

func (s *shareService) GetSharedByUser(ctx context.Context, email string) ([]SharedFileInfo, error) {
	rows, err := s.queries.GetFilesSharedByUser(ctx, sql.NullString{Valid: true, String: email})
	if err != nil {
		return nil, err
	}
	result := make([]SharedFileInfo, 0, len(rows))
	for _, r := range rows {
		result = append(result, SharedFileInfo{
			FileID:       r.FileID.Int32,
			OwnerID:      r.OwnerID.String(),
			FileName:     r.FileName,
			FileType:     r.FileType.String,
			MD5Checksum:  r.Md5Checksum,
			SharedBy:     r.SharedBy.String,
			SharedFor:    r.SharedFor.String,
			SharingToken: r.SharingToken,
			ExpiresAt:    r.ExpiresAt,
			Size:         r.Size.Int64,
		})
	}
	return result, nil
}

func (s *shareService) CountUnseen(ctx context.Context, email string) (int64, error) {
	return s.queries.CountUnseenShares(ctx, sql.NullString{Valid: true, String: email})
}

func (s *shareService) MarkSeen(ctx context.Context, email, token string) error {
	_, err := s.queries.MarkShareSeen(ctx, sqlc.MarkShareSeenParams{
		SharingToken: token,
		SharedFor:    sql.NullString{Valid: true, String: email},
	})
	return err
}

var ErrPasswordRequired = errors.New("share is password-protected")
var ErrWrongPassword = errors.New("wrong share password")
var ErrShareNotFound = errors.New("share not found or not owned by user")
var ErrPasswordTooLong = errors.New("password exceeds maximum length of 72 bytes")

const maxPasswordBytes = 72

func (s *shareService) shareBaseURL() string {
	if s.frontendEndpoint != "" {
		return s.frontendEndpoint
	}
	return s.backendEndpoint
}

func (s *shareService) GetPublicShareInfo(ctx context.Context, token string) (*ShareInfo, error) {
	row, err := s.queries.GetPublicShareMetadata(ctx, token)
	if err != nil {
		return nil, err
	}
	if row.ExpiresAt.Before(time.Now()) {
		return nil, ErrTokenExpired
	}
	if row.FailedAttempts >= maxShareFailedAttempts {
		return nil, ErrShareBlocked
	}
	return &ShareInfo{
		FileName:          row.FileName,
		ExpiresAt:         row.ExpiresAt,
		PasswordProtected: row.PasswordHash.String != "",
	}, nil
}

func verifySharePassword(provided, stored string) error {
	if stored == "" {
		return nil // not protected
	}
	if provided == "" {
		return ErrPasswordRequired
	}
	if err := bcrypt.CompareHashAndPassword([]byte(stored), []byte(provided)); err != nil {
		return ErrWrongPassword
	}
	return nil
}

func (s *shareService) ResolvePublicDownload(ctx context.Context, token, mode, password string) (*DownloadResult, error) {
	expiresAt, err := s.queries.GetTokenExpirationTime(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("checking token expiration: %w", err)
	}
	if expiresAt.Before(time.Now()) {
		return nil, ErrTokenExpired
	}

	_, err = s.queries.GetSharedFileIdFromToken(ctx, token)
	if err != nil {
		return nil, err
	}

	row, err := s.queries.GetBucketAndObjectFromToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("getting bucket/object for token: %w", err)
	}

	if row.FailedAttempts >= maxShareFailedAttempts {
		return nil, ErrShareBlocked
	}

	if err := verifySharePassword(password, row.PasswordHash.String); err != nil {
		if errors.Is(err, ErrWrongPassword) {
			newCount, incErr := s.queries.IncrementShareFailedAttempts(ctx, token)
			if incErr != nil {
				log.Printf("incrementing failed attempts for token %q: %v", token, incErr)
			}
			if newCount >= maxShareFailedAttempts {
				if _, delErr := s.queries.DeleteShareByToken(ctx, sqlc.DeleteShareByTokenParams{
					SharingToken: token,
					SharedBy:     row.SharedBy,
				}); delErr != nil {
					log.Printf("deleting blocked share %q: %v", token, delErr)
				}
				return nil, ErrShareBlocked
			}
		}
		return nil, err
	}

	url, err := s.buildDownloadURL(ctx, row.UserBucket.String, row.StorageMapping.String(), row.FileName, mode, time.Now().Add(time.Minute))
	if err != nil {
		return nil, fmt.Errorf("generating download URL: %w", err)
	}

	return &DownloadResult{
		URL:      url,
		FileName: row.FileName,
		UseProxy: mode == "download" && s.cloudFrontSigner != nil,
	}, nil
}

func (s *shareService) ResolvePersonalDownload(ctx context.Context, ownerID uuid.UUID, token, mode string) (*DownloadResult, error) {
	_, err := s.queries.GetFileIdFromToken(ctx, sql.NullString{Valid: true, String: token})
	if err != nil {
		return nil, err
	}

	row, err := s.queries.GetBucketObjectAndOwnerFromPrivateToken(ctx, sql.NullString{Valid: true, String: token})
	if err != nil {
		return nil, fmt.Errorf("getting bucket/object for private token: %w", err)
	}

	if ownerID != row.OwnerID {
		return nil, ErrAccessDenied
	}

	url, err := s.buildDownloadURL(ctx, row.BucketName.String, row.ObjectName.String(), row.FileName, mode, time.Now().Add(time.Minute))
	if err != nil {
		return nil, fmt.Errorf("generating download URL: %w", err)
	}

	return &DownloadResult{
		URL:      url,
		FileName: row.FileName,
		UseProxy: mode == "download" && s.cloudFrontSigner != nil,
	}, nil
}

// buildDownloadURL generates either a CloudFront signed URL or a storage presigned URL.
func (s *shareService) buildDownloadURL(ctx context.Context, bucket, object, filename, mode string, expiresAt time.Time) (string, error) {
	if s.cloudFrontSigner != nil {
		return s.cloudFrontSigner.SignURL(bucket, object, expiresAt)
	}
	var contentDisposition string
	if mode == "download" {
		contentDisposition = buildContentDisposition(filename)
	}
	return s.storage.GenerateSignedURL(ctx, bucket, object, expiresAt, contentDisposition)
}

func (s *shareService) sendSharingNotification(sharedByEmail, emailTo, expiryDate string, files []mailtypes.FileInfo) error {
	subject := fmt.Sprintf("Subject: New File Transfer from %s", sharedByEmail)
	mime := "\r\nMIME-version: 1.0;\r\nContent-Type: text/html; charset=\"UTF-8\";\r\n"

	htmlBody, err := templates.RenderMailTemplate("sharing", mailtypes.MailData{
		Files:       files,
		SenderEmail: sharedByEmail,
		ExpiryDate:  expiryDate,
	})
	if err != nil {
		return err
	}

	_, err = s.emailSender.Send(mailtypes.MessageConfig{
		From:    s.mailFrom,
		To:      []string{emailTo},
		Subject: subject,
		Mime:    mime,
		Body:    htmlBody,
	})
	return err
}

func (s *shareService) RevokeShare(ctx context.Context, token, ownerEmail string) error {
	n, err := s.queries.DeleteShareByToken(ctx, sqlc.DeleteShareByTokenParams{
		SharingToken: token,
		SharedBy:     sql.NullString{Valid: true, String: ownerEmail},
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrShareNotFound
	}
	return nil
}

func buildContentDisposition(filename string) string {
	safe := strings.TrimSpace(filename)
	safe = strings.ReplaceAll(safe, "\r", "")
	safe = strings.ReplaceAll(safe, "\n", "")
	safe = strings.ReplaceAll(safe, "\x00", "")
	if safe == "" {
		return "attachment"
	}
	v := mime.FormatMediaType("attachment", map[string]string{"filename": safe})
	if v == "" {
		return "attachment"
	}
	return v
}
