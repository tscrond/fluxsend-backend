package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	mailnotify "github.com/tscrond/fluxsend-backend/internal/mailservice/notify"
	mail "github.com/tscrond/fluxsend-backend/internal/mailservice/types"
	"github.com/tscrond/fluxsend-backend/internal/repo"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"github.com/tscrond/fluxsend-backend/pkg"
	"github.com/tscrond/fluxsend-backend/pkg/hash"
	"go.uber.org/zap"
)

type PasswordAuthService interface {
	RegisterUser(ctx context.Context, email, password string) error
	RegisterUserWithPasswordHash(ctx context.Context, email, passwordHash string) error
	LoginUser(ctx context.Context, email, password string) (string, error)
	SendConfirmationEmail(ctx context.Context, email, code, verifyLink string) error
	ConfirmEmail(ctx context.Context, email, code, challengeId string) error
	SendPasswordResetEmail(ctx context.Context, email, resetLink string) error
	CheckPasswordResetToken(ctx context.Context, email, purpose, token string) (string, error)
	CheckUserExists(ctx context.Context, email string) error
	CreatePasswordAuthChallenge(ctx context.Context, email, purpose, password string) (challengeID, code string, err error)
}

type passwordAuthService struct {
	log       *zap.SugaredLogger
	repo      repo.Repository
	notifier  mailnotify.Notifier
	dummyHash string
}

type registerChallengeContext struct {
	PasswordHash string `json:"password_hash"`
}

func NewPasswordAuthService(log *zap.SugaredLogger, emailSender mail.EmailSender, queries sqlc.Querier, repo repo.Repository, mailFrom string) PasswordAuthService {
	dummyHash, err := hash.HashPasswordArgon2id("fluxsend-dummy-password")
	if err != nil {
		log.Warnw("failed to precompute dummy argon2id hash", "error", err)
	}

	return &passwordAuthService{
		log:       log,
		repo:      repo,
		notifier:  mailnotify.NewMailNotifier(log, emailSender, mailFrom),
		dummyHash: dummyHash,
	}
}

func (s *passwordAuthService) CheckUserExists(ctx context.Context, email string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return errors.New("email is required")
	}

	_, err := s.repo.Queries().GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	return errors.New("user already exists")
}

func (s *passwordAuthService) RegisterUser(ctx context.Context, email, password string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || password == "" {
		return errors.New("email and password are required")
	}

	passwordHash, err := hash.HashPasswordArgon2id(password)
	if err != nil {
		s.log.Errorw("failed to hash password", "error", err)
		return err
	}

	return s.RegisterUserWithPasswordHash(ctx, email, passwordHash)
}

func (s *passwordAuthService) RegisterUserWithPasswordHash(ctx context.Context, email, passwordHash string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || passwordHash == "" {
		return errors.New("email and password hash are required")
	}

	tx, err := s.repo.BeginTx(ctx, nil)
	if err != nil {
		s.log.Errorw("failed to begin transaction", "error", err)
		return err
	}
	defer tx.Rollback()

	txq := s.repo.Queries().WithTx(tx)

	newUser, err := txq.CreateUser(ctx, email)
	if err != nil {
		s.log.Errorw("failed to create user", "error", err)
		return err
	}

	if _, err := txq.CreateIdentity(ctx, sqlc.CreateIdentityParams{
		UserID:         newUser.ID,
		Provider:       "password",
		Email:          sql.NullString{String: email, Valid: true},
		ProviderUserID: email,
		Name:           sql.NullString{String: strings.Split(email, "@")[0], Valid: true},
	}); err != nil {
		s.log.Errorw("failed to create identity", "error", err)
		return err
	}

	if _, err := txq.CreatePasswordCredentials(ctx, sqlc.CreatePasswordCredentialsParams{
		UserID:        newUser.ID,
		PasswordHash:  passwordHash,
		PasswordSetBy: "self_register",
	}); err != nil {
		s.log.Errorw("failed to create password credentials", "error", err)
		return err
	}

	if err := tx.Commit(); err != nil {
		s.log.Errorw("failed to commit transaction", "error", err)
		return err
	}

	return nil
}

func (s *passwordAuthService) LoginUser(ctx context.Context, email, password string) (string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || password == "" {
		return "", errors.New("invalid credentials")
	}

	queries := s.repo.Queries()
	user, userErr := queries.GetUserByEmail(ctx, email)

	hashToVerify := s.dummyHash
	hasCredentials := false

	if userErr == nil {
		credentials, err := queries.GetPasswordCredentialsByUserId(ctx, user.ID)
		if err == nil {
			hasCredentials = true
			hashToVerify = credentials.PasswordHash
		} else if !errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("loading password credentials: %w", err)
		}
	} else if !errors.Is(userErr, sql.ErrNoRows) {
		return "", fmt.Errorf("loading user by email: %w", userErr)
	}

	if hashToVerify == "" {
		fallbackHash, err := hash.HashPasswordArgon2id("fluxsend-dummy-password-fallback")
		if err == nil {
			hashToVerify = fallbackHash
		}
	}

	match, err := hash.VerifyPasswordArgon2id(password, hashToVerify)
	if err != nil {
		if userErr == nil && hasCredentials {
			return "", fmt.Errorf("verifying password: %w", err)
		}
		return "", errors.New("invalid credentials")
	}

	if userErr != nil || !hasCredentials || !match {
		return "", errors.New("invalid credentials")
	}

	return user.ID.String(), nil
}

func (s *passwordAuthService) CreatePasswordAuthChallenge(ctx context.Context, email, purpose, password string) (string, string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || purpose == "" {
		return "", "", errors.New("email and purpose are required")
	}

	if purpose == "register" && password == "" {
		return "", "", errors.New("password is required for register challenge")
	}

	if err := s.CheckUserExists(ctx, email); err != nil {
		return "", "", err
	}

	token := pkg.GenerateEmailConfirmationCode()
	tokenHash := hashToken(token)

	passwordHash, err := hash.HashPasswordArgon2id(password)
	if err != nil {
		return "", "", err
	}

	ctxPayload, err := json.Marshal(registerChallengeContext{PasswordHash: passwordHash})
	if err != nil {
		return "", "", fmt.Errorf("marshal register challenge context: %w", err)
	}

	challenge, err := s.repo.Queries().CreateEmailVerificationChallenge(ctx, sqlc.CreateEmailVerificationChallengeParams{
		Email:             email,
		UserID:            uuid.NullUUID{},
		Purpose:           purpose,
		CodeHash:          tokenHash,
		ExpiresAt:         time.Now().Add(5 * time.Minute),
		RequestedByIp:     pkg.GetClientIPFromContext(ctx),
		RequestContext:    ctxPayload,
		ResendAvailableAt: time.Now().Add(60 * time.Second),
	})
	if err != nil {
		s.log.Errorw("failed to create email verification challenge", "error", err)
		return "", "", err
	}

	return challenge.ID.String(), token, nil
}

func (s *passwordAuthService) SendConfirmationEmail(ctx context.Context, email, code, verifyLink string) error {
	if email == "" || code == "" {
		return nil
	}
	if err := s.notifier.SendConfirmationCode(email, code, verifyLink); err != nil {
		s.log.Errorw("failed to send confirmation email", "error", err)
		return err
	}
	return nil
}

func (s *passwordAuthService) ConfirmEmail(ctx context.Context, email, code, challengeId string) error {
	if email == "" || code == "" || challengeId == "" {
		return errors.New("email, code, and challengeId are required")
	}
	email = strings.TrimSpace(strings.ToLower(email))
	challengeIDParsed, err := uuid.Parse(challengeId)
	if err != nil {
		s.log.Errorw("failed to parse challengeId", "error", err)
		return err
	}

	emailVerification, err := s.repo.Queries().GetEmailVerificationChallengeById(ctx, challengeIDParsed)
	if err != nil {
		s.log.Errorw("failed to get email verification code", "error", err)
		return err
	}

	if emailVerification.Purpose != "register" {
		return errors.New("invalid challenge purpose")
	}

	if emailVerification.Email != email {
		return errors.New("challenge email mismatch")
	}

	if emailVerification.ConsumedAt.Valid {
		return errors.New("challenge already consumed")
	}

	if time.Now().After(emailVerification.ExpiresAt) {
		return errors.New("challenge expired")
	}

	if compareTokenWithHash(code, emailVerification.CodeHash) != nil {
		return fmt.Errorf("invalid confirmation code")
	}

	var regCtx registerChallengeContext
	if err := json.Unmarshal(emailVerification.RequestContext, &regCtx); err != nil {
		return fmt.Errorf("invalid challenge context: %w", err)
	}
	if regCtx.PasswordHash == "" {
		return errors.New("missing password hash in challenge context")
	}

	if err := s.CheckUserExists(ctx, email); err != nil {
		return err
	}

	if err := s.RegisterUserWithPasswordHash(ctx, email, regCtx.PasswordHash); err != nil {
		return err
	}

	if _, err := s.repo.Queries().ConsumeEmailVerificationChallenge(ctx, challengeIDParsed); err != nil {
		s.log.Errorw("failed to consume email verification challenge", "error", err)
		return err
	}

	return nil
}

func (s *passwordAuthService) SendPasswordResetEmail(ctx context.Context, email, resetLink string) error {
	if email == "" || resetLink == "" {
		return nil
	}
	if err := s.notifier.SendPasswordResetLink(email, resetLink); err != nil {
		s.log.Errorw("failed to send password reset email", "error", err)
		return err
	}
	return nil
}

func (s *passwordAuthService) CheckPasswordResetToken(ctx context.Context, email, purpose, token string) (string, error) {

	tokenFromDb, err := s.repo.Queries().GetEmailVerificationCode(ctx, sqlc.GetEmailVerificationCodeParams{
		Email:   email,
		Purpose: purpose,
	})
	if err != nil {
		s.log.Errorw("failed to get email verification code", "error", err)
		return "", err
	}

	if compareTokenWithHash(token, tokenFromDb.CodeHash) != nil {
		return "", fmt.Errorf("no token found for email: %s and purpose: %s", email, purpose)
	}

	return tokenFromDb.CodeHash, nil
}

func compareTokenWithHash(token, hash string) error {
	if token == "" || hash == "" {
		return fmt.Errorf("token or hash is empty")
	}

	tokenHashed := hashToken(token)

	if tokenHashed != hash {
		return fmt.Errorf("token does not match hash")
	}

	return nil
}

func hashToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}
