package api

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/tscrond/fluxsend-backend/internal/logger"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"github.com/tscrond/fluxsend-backend/internal/userdata"
)

var ErrFileLimitExceeded = errors.New("file limit exceeded")
var ErrStorageQuotaExceeded = errors.New("storage quota exceeded")
var ErrDailyUploadLimitExceeded = errors.New("daily upload limit exceeded")
var ErrDailyShareLimitExceeded = errors.New("daily share limit exceeded")

func (s *CoreHandlers) validateClassicUploadPlan(ctx context.Context, userUUID uuid.UUID, userPlan userdata.UserPlan) (exceedInfo map[string]any, err error) {
	log := logger.FromContext(ctx)
	quota, err := s.repository.Queries().CheckUploadQuota(ctx, sqlc.CheckUploadQuotaParams{
		OwnerID: userUUID,
		ID:      uuid.MustParse(userPlan.PlanID),
	})
	if err != nil {
		log.Errorw("DB error checking upload quota", "user", userUUID, "plan", userPlan.PlanName, "error", err)
		return nil, err
	}

	switch errType := s.checkForClassicPlanErrorType(quota); errType {
	case ErrFileLimitExceeded:
		log.Warnw("plan limit exceeded: max_files", "user", userUUID, "plan", userPlan.PlanName, "limit", userPlan.MaxFiles)
		return map[string]any{
			"msg":       "You have reached the maximum number of files for your plan",
			"max_files": userPlan.MaxFiles,
		}, errType
	case ErrStorageQuotaExceeded:
		log.Warnw("plan limit exceeded: max_total_storage_bytes", "user", userUUID, "plan", userPlan.PlanName, "limit", userPlan.MaxTotalStorageBytes)
		return map[string]any{
			"msg":                     "You have reached the maximum storage quota for your plan",
			"max_total_storage_bytes": userPlan.MaxTotalStorageBytes,
		}, errType
	case ErrDailyUploadLimitExceeded:
		log.Warnw("plan limit exceeded: max_files_sent_per_day", "user", userUUID, "plan", userPlan.PlanName, "limit", userPlan.MaxFilesSentPerDay)
		return map[string]any{
			"msg":               "You have reached the maximum number of daily uploads for your plan",
			"max_files_per_day": userPlan.MaxFilesSentPerDay,
		}, errType
	}
	return nil, nil
}

func (s *CoreHandlers) validateClassicSharePlan(ctx context.Context, sharedByEmail string, userPlan userdata.UserPlan) (exceedInfo map[string]any, err error) {
	log := logger.FromContext(ctx)
	sharesExceeded, err := s.repository.Queries().CheckShareQuota(ctx, sqlc.CheckShareQuotaParams{
		SharedBy: sql.NullString{String: sharedByEmail, Valid: true},
		ID:       uuid.MustParse(userPlan.PlanID),
	})
	if err != nil {
		log.Errorw("DB error checking share quota", "user", sharedByEmail, "plan", userPlan.PlanName, "error", err)
		return nil, err
	}

	if sharesExceeded {
		log.Warnw("plan limit exceeded: max_shares_per_day", "user", sharedByEmail, "plan", userPlan.PlanName, "limit", userPlan.MaxSharesPerDay)
		return map[string]any{
			"msg":                "You have reached the maximum number of daily shares for your plan",
			"max_shares_per_day": userPlan.MaxSharesPerDay,
		}, ErrDailyShareLimitExceeded
	}
	return nil, nil
}

func (s *CoreHandlers) checkForClassicPlanErrorType(quota sqlc.CheckUploadQuotaRow) error {
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
