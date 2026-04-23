package api

import (
	"database/sql"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/tscrond/fluxsend-backend/internal/service"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
)

func (s *APIServer) downloadThroughProxyPersonal(w http.ResponseWriter, r *http.Request) {
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
		if errors.Is(err, sql.ErrNoRows) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "file_does_not_exist", "")
			return
		}
		if errors.Is(err, service.ErrAccessDenied) {
			pkg.WriteJSONResponse(w, http.StatusForbidden, "access_denied", "")
			return
		}
		log.Println("ResolvePersonalDownload error:", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "cannot_get_bucket_data", "")
		return
	}

	s.handleDownloadResponse(w, r, result.URL, result.FileName, mode)
}

func (s *APIServer) downloadThroughProxy(w http.ResponseWriter, r *http.Request) {
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

	result, err := s.shares.ResolvePublicDownload(r.Context(), token, mode)
	if err != nil {
		if errors.Is(err, service.ErrTokenExpired) {
			pkg.WriteJSONResponse(w, http.StatusForbidden, "past_expiration_time_or_does_not_exist", "")
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			pkg.WriteJSONResponse(w, http.StatusNotFound, "token_does_not_exist", "")
			return
		}
		log.Println("ResolvePublicDownload error:", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	s.handleDownloadResponse(w, r, result.URL, result.FileName, mode)
}

func (s *APIServer) handleDownloadResponse(w http.ResponseWriter, r *http.Request, signedUrl, filename, mode string) {
	if mode == "download" && s.cloudFrontSigner != nil {
		s.proxyDownload(w, signedUrl, filename)
		return
	}
	http.Redirect(w, r, signedUrl, http.StatusFound)
}

func (s *APIServer) proxyDownload(w http.ResponseWriter, signedUrl, filename string) {
	resp, err := http.Get(signedUrl)
	if err != nil {
		log.Printf("download proxy: fetch error: %v", err)
		pkg.WriteJSONResponse(w, http.StatusBadGateway, "download_proxy_error", "")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("download proxy: upstream returned %d", resp.StatusCode)
		pkg.WriteJSONResponse(w, http.StatusBadGateway, "download_proxy_error", "")
		return
	}

	w.Header().Set("Content-Disposition", buildAttachmentContentDisposition(filename))
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		w.Header().Set("Content-Length", cl)
	}

	io.Copy(w, resp.Body)
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

func (s *APIServer) getDataSharedForUser(w http.ResponseWriter, r *http.Request) {
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
		log.Println("GetSharedForUser error:", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"files": files,
	})
}

func (s *APIServer) getDataSharedByUser(w http.ResponseWriter, r *http.Request) {
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
		log.Println("GetSharedByUser error:", err)
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "", map[string]any{
		"files": files,
	})
}
