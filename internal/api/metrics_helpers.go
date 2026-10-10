package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/tscrond/fluxsend-backend/internal/service"
)

type cliRouteDomainContextKey struct{}

var cliRouteDomainCtxKey = cliRouteDomainContextKey{}

func withCLIRouteDomain(domain routeDomain) routeMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), cliRouteDomainCtxKey, domain)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func cliRouteMetricDomain(ctx context.Context) string {
	if domain, ok := ctx.Value(cliRouteDomainCtxKey).(routeDomain); ok {
		return string(domain)
	}
	return string(routeDomainPrivate)
}

func (s *CoreHandlers) recordQuotaRejection(err error) {
	switch {
	case errors.Is(err, ErrFileLimitExceeded):
		s.metrics.RecordQuotaRejection("files")
	case errors.Is(err, ErrStorageQuotaExceeded):
		s.metrics.RecordQuotaRejection("storage")
	case errors.Is(err, ErrDailyUploadLimitExceeded):
		s.metrics.RecordQuotaRejection("daily_uploads")
	case errors.Is(err, ErrDailyShareLimitExceeded):
		s.metrics.RecordQuotaRejection("daily_shares")
	case errors.Is(err, ErrWorkspaceFilesLimitExceeded):
		s.metrics.RecordQuotaRejection("workspace_files")
	case errors.Is(err, ErrWorkspaceStorageLimitExceeded):
		s.metrics.RecordQuotaRejection("workspace_storage")
	case errors.Is(err, ErrWorkspaceFoldersLimitExceeded):
		s.metrics.RecordQuotaRejection("workspace_folders")
	case errors.Is(err, ErrWorkspaceUsersLimitExceeded):
		s.metrics.RecordQuotaRejection("workspace_members")
	case errors.Is(err, ErrWorkspacesPerUserLimitExceeded):
		s.metrics.RecordQuotaRejection("workspaces")
	case errors.Is(err, ErrPrivateAPIKeysLimitExceeded):
		s.metrics.RecordQuotaRejection("private_api_keys")
	case errors.Is(err, ErrWorkspaceAPIKeysLimitExceeded):
		s.metrics.RecordQuotaRejection("workspace_api_keys")
	}
}

func (s *CoreHandlers) recordShareCreatedN(shareType string, count int) {
	for i := 0; i < count; i++ {
		s.metrics.RecordShareCreated(shareType)
	}
}

func downloadOutcomeFromError(err error) string {
	switch {
	case err == nil:
		return "success"
	case errors.Is(err, sql.ErrNoRows), errors.Is(err, service.ErrWsFileNotFound):
		return "not_found"
	case errors.Is(err, service.ErrTokenExpired),
		errors.Is(err, service.ErrAccessDenied),
		errors.Is(err, service.ErrPasswordRequired),
		errors.Is(err, service.ErrWrongPassword),
		errors.Is(err, service.ErrShareBlocked):
		return "denied"
	default:
		return "error"
	}
}

func resolvedURLMechanism(hasCloudFront bool) string {
	if hasCloudFront {
		return "cloudfront"
	}
	return "signed_url"
}

func servedDownloadMechanism(mode string, useProxy bool, hasCloudFront bool) string {
	if mode == "download" && useProxy {
		return "proxy"
	}
	if hasCloudFront {
		return "cloudfront"
	}
	return "signed_url"
}

func workspaceDownloadMechanism(mode string, hasCloudFront bool) string {
	if mode == "download" && hasCloudFront {
		return "proxy"
	}
	return "signed_url"
}
