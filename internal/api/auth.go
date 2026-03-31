package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/tscrond/fluxsend-backend/internal/mappings"
	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"github.com/tscrond/fluxsend-backend/internal/userdata"
	pkg "github.com/tscrond/fluxsend-backend/pkg"
	"golang.org/x/oauth2"
)

const (
	IsProd = true
)

func (s *APIServer) oauthHandler(w http.ResponseWriter, r *http.Request) {
	url := s.OAuthConfig.AuthCodeURL("state", oauth2.AccessTypeOffline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func (s *APIServer) authCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	oauthErr := r.URL.Query().Get("error")
	if oauthErr != "" {
		oauthErrDescription := r.URL.Query().Get("error_description")
		log.Printf("oauth callback authorization error: %s (%s)", oauthErr, oauthErrDescription)
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", fmt.Sprintf("OAuth authorization failed: %s", oauthErr))
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "Missing authorization code in callback")
		return
	}

	t, err := s.OAuthConfig.Exchange(ctx, code)
	if err != nil {
		log.Printf("oauth token exchange failed: %v", err)
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "OAuth token exchange failed")
		return
	}

	client := s.OAuthConfig.Client(ctx, t)

	// Getting the user public details from google API endpoint
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		log.Printf("google userinfo request failed: %v", err)
		pkg.WriteJSONResponse(w, http.StatusBadGateway, "", "Could not fetch Google user profile")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		log.Printf("google userinfo request returned status=%d body=%s", resp.StatusCode, string(responseBody))
		pkg.WriteJSONResponse(w, http.StatusBadGateway, "", "Could not fetch Google user profile")
		return
	}

	var jsonResp userdata.AuthorizedUserInfo

	// Reading the JSON body using JSON decoder
	err = json.NewDecoder(resp.Body).Decode(&jsonResp)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "", "Error decoding JSON response")
		return
	}

	// fmt.Printf("%+v", jsonResp)

	// Store user information in a session (cookie)
	sessionCookie := &http.Cookie{
		Name:     "access_token",
		Value:    fmt.Sprintf("%s", t.AccessToken),
		HttpOnly: true,
		Secure:   IsProd,
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour),
	}
	http.SetCookie(w, sessionCookie)

	// create/update new user if not exists
	username := sql.NullString{String: jsonResp.Name, Valid: true}
	userBucket := sql.NullString{
		String: fmt.Sprintf("%s-%s", s.bucketHandler.GetBucketBaseName(), jsonResp.Id),
		Valid: func() bool {
			return jsonResp.Id != ""
		}(),
	}
	if err := s.repository.Queries.CreateUser(ctx, sqlc.CreateUserParams{
		GoogleID:   jsonResp.Id,
		UserName:   username,
		UserEmail:  jsonResp.Email,
		UserBucket: userBucket,
	}); err != nil {
		http.Redirect(w, r, s.backendConfig.FrontendEndpoint, http.StatusInternalServerError)
	}

	log.Printf("USER ID: %s", jsonResp.Id)
	if err := s.syncDatabaseWithBucket(ctx, jsonResp.Id); err != nil {
		log.Println("error syncing the DB: ", err)
	} else {
		log.Println("database sync with remote buckets succeeded!")
	}

	http.Redirect(w, r, s.backendConfig.FrontendEndpoint, http.StatusTemporaryRedirect)
}

func (s *APIServer) syncDatabaseWithBucket(ctx context.Context, googleUserID string) error {
	// sync strategy:
	// 1. check objects in db
	// 2. check objects in GCS
	// 3. diff GCS to DB
	// 4. fill the DB with diff between GCS
	// parse data of logged in user

	// 1. check objects in the db
	filesFromDatabase, err := s.repository.Queries.GetFilesByOwner(
		ctx,
		sql.NullString{Valid: true, String: googleUserID},
	)
	if err != nil {
		return err
	}

	// log.Println(filesFromDatabase)

	// 2. get files objects from bucket handler
	bucketDataFromObjectStore, err := s.bucketHandler.GetUserBucketData(ctx, googleUserID)
	if err != nil {
		return err
	}

	// 3. map any type to *mappings.BucketData
	bucketDataMapped, ok := bucketDataFromObjectStore.(*mappings.BucketData)
	if !ok {
		return errors.New("cannot map bucket data")
	}

	// 4. transform mapped data to []sqlc.File format
	filesFromBuckets, err := mappings.MapBucketDataToDBFormat(googleUserID, bucketDataMapped)
	if err != nil {
		return errors.New("cannot map bucket data to db format")
	}

	// 5. check if the DB has missing records, if yes - return them as []sqlc.File
	diffFiles := mappings.FindMissingFilesFromDB(filesFromBuckets, filesFromDatabase)
	// log.Printf("missing files in the DB %+v\n", diffFiles)

	// 6. fill the missing records
	for _, f := range diffFiles {
		insertArgs := sqlc.InsertFileParams{
			OwnerGoogleID:        f.OwnerGoogleID,
			FileName:             f.FileName,
			FileType:             f.FileType,
			Size:                 f.Size,
			Md5Checksum:          f.Md5Checksum,
			PrivateDownloadToken: f.PrivateDownloadToken,
		}

		_, err := s.repository.Queries.InsertFile(ctx, insertArgs)
		if err != nil {
			log.Println("errors syncing the DB (filling missing records): ", err)
			continue
		}

	}
	return nil
}

func (s *APIServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("access_token")
		// fmt.Println(cookie)
		if err != nil || cookie.Value == "" {
			pkg.WriteJSONResponse(w, http.StatusForbidden, "", "Unauthorized")
			return
		}

		valid, verifiedUserData := s.verifyToken(cookie.Value)
		if !valid {
			pkg.WriteJSONResponse(w, http.StatusForbidden, "", "Unauthorized (invalid or expired session)")
			return
		}
		// log.Println("verified user:", verifiedUserData)

		userInfo, err := s.fetchUserInfo(cookie.Value)
		if err != nil {
			pkg.WriteJSONResponse(w, http.StatusForbidden, "", "Could not fetch logged user info")
			return
		}
		// log.Println("logged user info::", userInfo)

		ctx := context.WithValue(r.Context(), userdata.VerifiedUserContextKey, verifiedUserData)
		ctx = context.WithValue(ctx, userdata.AuthorizedUserContextKey, userInfo)

		if err := s.bucketHandler.CreateBucketIfNotExists(ctx, userInfo.Id); err != nil {
			log.Println(err)
			return
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *APIServer) verifyToken(cookieValue string) (bool, *userdata.VerifiedUserInfo) {
	resp, err := http.Get(fmt.Sprintf("https://www.googleapis.com/oauth2/v3/tokeninfo?access_token=%s", cookieValue))
	if err != nil {
		log.Println(err)
		return false, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Println("Token verification failed, invalid token")
		return false, nil
	}

	var userInfo userdata.VerifiedUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		log.Println("cannot decode user info")
		return false, nil
	}

	if userInfo.Email == "" || userInfo.Sub == "" {
		log.Println("Invalid token: Missing email or user ID")
		return false, nil
	}

	return true, &userInfo
}

// Revoke OAuth2 token and expire session cookie
func (s *APIServer) logout(w http.ResponseWriter, r *http.Request) {
	// Check if access_token cookie exists
	cookie, err := r.Cookie("access_token")
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusNotFound, "cookie_not_found", map[string]any{
			"logout_successful": true,
		})
		return
	}

	// Prepare request to revoke OAuth2 token
	revokeURL := "https://oauth2.googleapis.com/revoke"
	formData := url.Values{}
	formData.Set("token", cookie.Value)

	req, err := http.NewRequest("POST", revokeURL, nil)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "logout_error", map[string]any{
			"logout_successful": false,
		})
		return
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.URL.RawQuery = formData.Encode() // Send token in body

	// Send request
	client := http.DefaultClient
	resp, err := client.Do(req)
	if err != nil {
		pkg.WriteJSONResponse(w, http.StatusInternalServerError, "logout_error", map[string]any{
			"logout_successful": false,
		})
		return
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		pkg.WriteJSONResponse(w, resp.StatusCode, "failed_to_revoke_token", map[string]any{
			"logout_successful": false,
		})
		return
	}

	// Expire session cookie
	expiredCookie := &http.Cookie{
		Name:     "access_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Expires:  time.Unix(0, 0), // Expire immediately
		MaxAge:   -1,              // Remove from browser
	}

	http.SetCookie(w, expiredCookie)

	w.WriteHeader(http.StatusOK)
	// Return success response
	pkg.WriteJSONResponse(w, http.StatusOK, "session_invalidated", map[string]any{
		"logout_successful": true,
	})
}

func (s *APIServer) isValid(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		pkg.WriteJSONResponse(w, http.StatusBadRequest, "bad_request", map[string]any{
			"authenticated": false,
			"user_info":     nil,
		})
		return
	}
	cookie, err := r.Cookie("access_token")
	if err != nil || cookie.Value == "" {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "access_denied", map[string]any{
			"authenticated": false,
			"user_info":     nil,
		})
		return
	}

	// fmt.Println(cookie.Value)

	valid, userInfo := s.verifyToken(cookie.Value)
	if !valid {
		pkg.WriteJSONResponse(w, http.StatusForbidden, "access_denied", map[string]any{
			"authenticated": false,
			"user_info":     nil,
		})
		return
	}

	pkg.WriteJSONResponse(w, http.StatusOK, "access_granted", map[string]any{
		"authenticated": true,
		"user_info":     userInfo,
	})
}

func (s *APIServer) fetchUserInfo(accessToken string) (*userdata.AuthorizedUserInfo, error) {
	// Call Google’s userinfo API
	req, err := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	if err != nil {
		return nil, err
	}

	// Add Authorization header
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := http.DefaultClient
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Decode JSON response
	var user userdata.AuthorizedUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}

	return &user, nil
}
