package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/tscrond/fluxsend-backend/internal/logger"
	"github.com/tscrond/fluxsend-backend/internal/service"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

// downloadThroughProxyPersonal resolves a private download token for the owner.
// @Summary Download private file
// @Description Resolves a private download token for the authenticated file owner and redirects to the download URL.
// @Tags Downloads
// @Param token path string true "Download token"
// @Param mode query string false "inline or download"
// @Success 302 "Redirect to the signed URL"
// @Failure 401 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /api/d/private/{token} [get]
func (s *CoreHandlers) downloadThroughProxyPersonal(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", "")
		return
	}

	_, ownerID, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "authorization_failed", "")
		return
	}

	token := chi.URLParam(r, "token")
	if token == "" {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "empty_token", "")
		return
	}

	mode := r.URL.Query().Get("mode")
	if mode != "inline" && mode != "download" && mode != "" {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "invalid_download_mode", "")
		return
	}

	result, err := s.shares.ResolvePersonalDownload(r.Context(), ownerID, token, mode)
	if err != nil {
		s.metrics.RecordDownload("private", servedDownloadMechanism(mode, mode == "download" && s.cloudFrontSigner != nil, s.cloudFrontSigner != nil), downloadOutcomeFromError(err))
		if errors.Is(err, sql.ErrNoRows) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "file_does_not_exist", "")
			return
		}
		if errors.Is(err, service.ErrAccessDenied) {
			pkg.WriteJSONResponse(w, http.StatusForbidden, "access_denied", "")
			return
		}
		log.Errorw("ResolvePersonalDownload error", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "cannot_get_bucket_data", "")
		return
	}

	mechanism, responseErr := s.handleDownloadResponse(w, r, result.URL, result.FileName, mode)
	s.metrics.RecordDownload("private", mechanism, downloadOutcomeFromError(responseErr))
}

// publicShareInfo returns metadata for a public share token.
// @Summary Get public share info
// @Description Returns public metadata for a share token without requiring authentication.
// @Tags Sharing
// @Param token path string true "Share token"
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Router /api/share/info/{token} [get]
func (s *CoreHandlers) publicShareInfo(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "method_not_allowed", "")
		return
	}
	token := chi.URLParam(r, "token")
	if token == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "empty_token", "")
		return
	}

	info, err := s.shares.GetPublicShareInfo(r.Context(), token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "token_does_not_exist", "")
			return
		}
		if errors.Is(err, service.ErrTokenExpired) {
			pkg.WriteJSONResponse(w, http.StatusForbidden, "past_expiration_time_or_does_not_exist", "")
			return
		}
		if errors.Is(err, service.ErrShareBlocked) {
			pkg.WriteJSONResponse(w, http.StatusForbidden, "share_blocked", "")
			return
		}
		log.Errorw("publicShareInfo error", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", info)
}

// resolvePublicShare validates a public share token and returns a signed URL.
// @Summary Resolve public share
// @Description Resolves a public share token and returns a signed download URL.
// @Tags Sharing
// @Accept json
// @Produce json
// @Param token path string true "Share token"
// @Param mode query string false "inline or download"
// @Param request body object false "Optional password payload"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /api/share/resolve/{token} [post]
func (s *CoreHandlers) resolvePublicShare(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodPost {
		pkg.WriteJSONResponse(w, http.StatusMethodNotAllowed, "method_not_allowed", "")
		return
	}
	token := chi.URLParam(r, "token")
	if token == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "empty_token", "")
		return
	}

	mode := r.URL.Query().Get("mode")
	if mode != "inline" && mode != "download" && mode != "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_download_mode", "")
		return
	}

	var body struct {
		Password string `json:"password"`
	}
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "invalid_json", "")
		return
	}

	result, err := s.shares.ResolvePublicDownload(r.Context(), token, mode, body.Password)
	if err != nil {
		s.metrics.RecordDownload("public", resolvedURLMechanism(s.cloudFrontSigner != nil), downloadOutcomeFromError(err))
		if errors.Is(err, service.ErrTokenExpired) {
			pkg.WriteJSONResponse(w, http.StatusForbidden, "past_expiration_time_or_does_not_exist", "")
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "token_does_not_exist", "")
			return
		}
		if errors.Is(err, service.ErrPasswordRequired) {
			pkg.WriteJSONResponse(w, http.StatusUnauthorized, "password_required", "")
			return
		}
		if errors.Is(err, service.ErrWrongPassword) {
			pkg.WriteJSONResponse(w, http.StatusForbidden, "wrong_password", "")
			return
		}
		if errors.Is(err, service.ErrShareBlocked) {
			pkg.WriteJSONResponse(w, http.StatusForbidden, "share_blocked", "")
			return
		}
		log.Errorw("resolvePublicShare error", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		return
	}
	s.metrics.RecordDownload("public", resolvedURLMechanism(s.cloudFrontSigner != nil), "success")

	// When storage is internal-only, hand the client the backend's own download
	// endpoint instead of a signed URL that only resolves inside the network.
	downloadURL := result.URL
	if s.proxyDownloads {
		downloadURL = s.publicDownloadURL(token, mode)
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]string{
		"url":       downloadURL,
		"file_name": result.FileName,
	})
}

// publicDownloadURL builds the app-origin URL that streams a shared object
// through the backend.
func (s *CoreHandlers) publicDownloadURL(token, mode string) string {
	target := strings.TrimSuffix(s.backendConfig.BackendEndpoint, "/") + "/d/" + url.PathEscape(token)
	if mode != "" {
		target += "?mode=" + url.QueryEscape(mode)
	}
	return target
}

// downloadThroughProxy resolves a public share token and returns the file content.
// @Summary Download public share
// @Description Resolves a public share token and either redirects or proxies the file content.
// @Tags Downloads
// @Param token path string true "Share token"
// @Param mode query string false "inline or download"
// @Success 302 "Redirect to the signed URL"
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /api/d/{token} [get]
func (s *CoreHandlers) downloadThroughProxy(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", "")
		return
	}

	mode := r.URL.Query().Get("mode")
	if mode != "inline" && mode != "download" && mode != "" {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "invalid_download_mode", "")
		return
	}

	token := chi.URLParam(r, "token")

	var passwordResult struct {
		Password string `json:"password"`
	}
	// Decode is intentionally lenient: an empty body (non-password-protected share)
	// returns io.EOF which we ignore; password stays "" and verifySharePassword handles it.
	_ = json.NewDecoder(r.Body).Decode(&passwordResult)
	defer r.Body.Close()

	result, err := s.shares.ResolvePublicDownload(r.Context(), token, mode, passwordResult.Password)
	if err != nil {
		s.metrics.RecordDownload("public", servedDownloadMechanism(mode, mode == "download" && s.cloudFrontSigner != nil, s.cloudFrontSigner != nil), downloadOutcomeFromError(err))
		if errors.Is(err, service.ErrTokenExpired) {
			pkg.WriteJSONResponse(w, http.StatusForbidden, "past_expiration_time_or_does_not_exist", "")
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "token_does_not_exist", "")
			return
		}
		if errors.Is(err, service.ErrPasswordRequired) {
			pkg.WriteJSONResponse(w, http.StatusUnauthorized, "password_required", "")
			return
		}
		if errors.Is(err, service.ErrWrongPassword) {
			pkg.WriteJSONResponse(w, http.StatusForbidden, "wrong_password", "")
			return
		}
		if errors.Is(err, service.ErrShareBlocked) {
			pkg.WriteJSONResponse(w, http.StatusForbidden, "share_blocked", "")
			return
		}
		log.Errorw("ResolvePublicDownload error", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	mechanism, responseErr := s.handleDownloadResponse(w, r, result.URL, result.FileName, mode)
	s.metrics.RecordDownload("public", mechanism, downloadOutcomeFromError(responseErr))
}

func (s *CoreHandlers) handleDownloadResponse(w http.ResponseWriter, r *http.Request, signedUrl, filename, mode string) (string, error) {
	// Stream through the backend when the storage endpoint is internal-only or
	// when CloudFront attachment downloads are enabled.
	if s.proxyDownloads || (mode == "download" && s.cloudFrontSigner != nil) {
		return "proxy", s.proxyDownload(w, r, signedUrl, filename, mode)
	}
	if s.cloudFrontSigner != nil {
		http.Redirect(w, r, signedUrl, http.StatusFound)
		return "cloudfront", nil
	}
	http.Redirect(w, r, signedUrl, http.StatusFound)
	return "signed_url", nil
}

func (s *CoreHandlers) proxyDownload(w http.ResponseWriter, r *http.Request, signedUrl, filename, mode string) error {
	log := logger.FromContext(r.Context())

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, signedUrl, nil)
	if err != nil {
		log.Errorw("download proxy: request error", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "download_proxy_error", "")
		return err
	}
	// Keep range requests working so inline media can seek.
	if rng := r.Header.Get("Range"); rng != "" {
		req.Header.Set("Range", rng)
	}
	if ifRange := r.Header.Get("If-Range"); ifRange != "" {
		req.Header.Set("If-Range", ifRange)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Errorw("download proxy: fetch error", "error", err)
		pkg.WriteJSONResponse(w, http.StatusBadGateway, "download_proxy_error", "")
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		log.Warnw("download proxy: upstream error", "status", resp.StatusCode)
		pkg.WriteJSONResponse(w, http.StatusBadGateway, "download_proxy_error", "")
		return errors.New("download_proxy_error")
	}

	if mode == "download" {
		w.Header().Set("Content-Disposition", buildAttachmentContentDisposition(filename))
	}
	// The object is now served from the application origin, so sandbox the
	// response and disable MIME sniffing to prevent stored XSS from uploaded
	// HTML/SVG being viewed inline.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox")
	for _, header := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if value := resp.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}

	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Warnw("download proxy: stream error", "error", err)
		return err
	}
	return nil
}

func buildAttachmentContentDisposition(filename string) string {
	safeFilename := sanitizeDownloadFilename(filename)
	headerValue := mime.FormatMediaType("attachment", map[string]string{"filename": safeFilename})
	if headerValue == "" {
		return "attachment"
	}
	return headerValue
}

func sanitizeDownloadFilename(filename string) string {
	name := strings.TrimSpace(filename)
	name = strings.ReplaceAll(name, "\r", "")
	name = strings.ReplaceAll(name, "\n", "")
	name = strings.ReplaceAll(name, "\x00", "")
	if name == "" {
		return "download"
	}
	return name
}

// getDataSharedForUser lists files shared with the authenticated user.
// @Summary List received shares
// @Description Returns all files shared with the current user.
// @Tags Sharing
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Router /api/files/received [get]
func (s *CoreHandlers) getDataSharedForUser(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", "")
		return
	}

	authUser, _, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "authorization_failed", "")
		return
	}

	files, err := s.shares.GetSharedForUser(r.Context(), authUser.Email)
	if err != nil {
		log.Errorw("GetSharedForUser error", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"files": files,
	})
}

// getDataSharedByUser lists files shared by the authenticated user.
// @Summary List outgoing shares
// @Description Returns all files that the current user has shared with others.
// @Tags Sharing
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Router /api/files/shared_by_user [get]
func (s *CoreHandlers) getDataSharedByUser(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", "")
		return
	}

	authUser, _, ok := parseAuthorizedUserUUID(r)
	if !ok {
		pkg.WriteJSONResponse(w, http.StatusUnauthorized, "authorization_failed", "")
		return
	}

	files, err := s.shares.GetSharedByUser(r.Context(), authUser.Email)
	if err != nil {
		log.Errorw("GetSharedByUser error", "error", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"files": files,
	})
}
