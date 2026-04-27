package api

import (
	"context"
	"database/sql"
	"errors"
	"log"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"github.com/tscrond/fluxsend-backend/internal/userdata"
)

var ErrFileLimitExceeded = errors.New("file limit exceeded")
var ErrStorageQuotaExceeded = errors.New("storage quota exceeded")
var ErrDailyUploadLimitExceeded = errors.New("daily upload limit exceeded")
var ErrDailyShareLimitExceeded = errors.New("daily share limit exceeded")

func (s *APIServer) validatePlan(ctx context.Context, userUUID uuid.UUID, userPlan userdata.UserPlan) (exceedInfo map[string]any, err error) {
	quota, err := s.repository.Queries().CheckUploadQuota(ctx, sqlc.CheckUploadQuotaParams{
		OwnerID: userUUID,
		ID:      uuid.MustParse(userPlan.PlanID),
	})
	if err != nil {
		log.Printf("[plan-limit] DB error checking upload quota for user=%s plan=%s: %v", userUUID, userPlan.PlanName, err)
		return nil, err
	}

	switch errType := s.checkForPlanErrorType(quota); errType {
	case ErrFileLimitExceeded:
		log.Printf("[plan-limit] user=%s plan=%s limit=max_files value=%d: file limit exceeded", userUUID, userPlan.PlanName, userPlan.MaxFiles)
		return map[string]any{
			"msg":       "You have reached the maximum number of files for your plan",
			"max_files": userPlan.MaxFiles,
		}, errType
	case ErrStorageQuotaExceeded:
		log.Printf("[plan-limit] user=%s plan=%s limit=max_total_storage_bytes value=%d: storage quota exceeded", userUUID, userPlan.PlanName, userPlan.MaxTotalStorageBytes)
		return map[string]any{
			"msg":                     "You have reached the maximum storage quota for your plan",
			"max_total_storage_bytes": userPlan.MaxTotalStorageBytes,
		}, errType
	case ErrDailyUploadLimitExceeded:
		log.Printf("[plan-limit] user=%s plan=%s limit=max_files_sent_per_day value=%d: daily upload limit exceeded", userUUID, userPlan.PlanName, userPlan.MaxFilesSentPerDay)
		return map[string]any{
			"msg":               "You have reached the maximum number of daily uploads for your plan",
			"max_files_per_day": userPlan.MaxFilesSentPerDay,
		}, errType
	}
	return nil, nil
}

func (s *APIServer) validateSharePlan(ctx context.Context, sharedByEmail string, userPlan userdata.UserPlan) (exceedInfo map[string]any, err error) {
	sharesExceeded, err := s.repository.Queries().CheckShareQuota(ctx, sqlc.CheckShareQuotaParams{
		SharedBy: sql.NullString{String: sharedByEmail, Valid: true},
		ID:       uuid.MustParse(userPlan.PlanID),
	})
	if err != nil {
		log.Printf("[plan-limit] DB error checking share quota for user=%s plan=%s: %v", sharedByEmail, userPlan.PlanName, err)
		return nil, err
	}

	if sharesExceeded {
		log.Printf("[plan-limit] user=%s plan=%s limit=max_shares_per_day value=%d: daily share limit exceeded", sharedByEmail, userPlan.PlanName, userPlan.MaxSharesPerDay)
		return map[string]any{
			"msg":                "You have reached the maximum number of daily shares for your plan",
			"max_shares_per_day": userPlan.MaxSharesPerDay,
		}, ErrDailyShareLimitExceeded
	}
	return nil, nil
}

func (s *APIServer) checkForPlanErrorType(quota sqlc.CheckUploadQuotaRow) error {
	switch {
	case quota.FilesExceeded:
		return ErrFileLimitExceeded
	case quota.StorageExceeded:
		return ErrStorageQuotaExceeded
	case quota.DailyExceeded:
		return ErrDailyUploadLimitExceeded
	}
	return nil
}
