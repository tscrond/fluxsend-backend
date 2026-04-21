//go:build ignore

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

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
	// Bucket name will be updated with UUID after user creation
	userBucket := sql.NullString{
		String: fmt.Sprintf("%s-%s", s.bucketHandler.GetBucketBaseName(), jsonResp.Id),
		Valid: func() bool {
			return jsonResp.Id != ""
		}(),
	}
	createdUser, err := s.repository.Queries.CreateUser(ctx, sqlc.CreateUserParams{
		GoogleID:   jsonResp.Id,
		UserName:   username,
		UserEmail:  jsonResp.Email,
		UserBucket: userBucket,
	})
	if err != nil {
		log.Printf("error creating/updating user: %v", err)
		http.Redirect(w, r, s.backendConfig.FrontendEndpoint, http.StatusInternalServerError)
		return
	}

	internalID := createdUser.ID.String()

	// Update bucket name to use UUID if it still has the old google_id format
	expectedBucket := fmt.Sprintf("%s-%s", s.bucketHandler.GetBucketBaseName(), internalID)
	if createdUser.UserBucket.String != expectedBucket {
		if err := s.repository.Queries.UpdateUserBucketNameById(ctx, sqlc.UpdateUserBucketNameByIdParams{
			UserBucket: sql.NullString{String: expectedBucket, Valid: true},
			ID:         createdUser.ID,
		}); err != nil {
			log.Printf("error updating bucket name: %v", err)
		}
	}

	log.Printf("USER ID (UUID): %s (Google: %s)", internalID, jsonResp.Id)
	if err := s.syncDatabaseWithBucket(ctx, internalID); err != nil {
		log.Println("error syncing the DB: ", err)
	} else {
		log.Println("database sync with remote buckets succeeded!")
	}

	http.Redirect(w, r, s.backendConfig.FrontendEndpoint, http.StatusTemporaryRedirect)
}

// func (s *APIServer) syncDatabaseWithBucket(ctx context.Context, userUUID string) error {
// 	parsedUUID, err := uuid.Parse(userUUID)
// 	if err != nil {
// 		return fmt.Errorf("invalid user UUID: %w", err)
// 	}

// 	filesFromDatabase, err := s.repository.Queries.GetFilesByOwner(
// 		ctx,
// 		parsedUUID,
// 	)
// 	if err != nil {
// 		return err
// 	}

// 	// The DB is the source of truth for visible names now because bucket object
// 	// keys are anonymous UUIDs and cannot be mapped back to logical paths.
// 	log.Printf("database metadata sync complete for user=%s files=%d", parsedUUID.String(), len(filesFromDatabase))

// 	return nil
// }

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

		// Look up internal UUID from google_id
		dbUser, err := s.repository.Queries.GetUserByGoogleID(r.Context(), userInfo.Id)
		if err != nil {
			log.Printf("cannot find user by google_id %s: %v", userInfo.Id, err)
			pkg.WriteJSONResponse(w, http.StatusForbidden, "", "User not found")
			return
		}
		userInfo.InternalID = dbUser.ID.String()

		ctx := context.WithValue(r.Context(), userdata.VerifiedUserContextKey, verifiedUserData)
		ctx = context.WithValue(ctx, userdata.AuthorizedUserContextKey, userInfo)

		if err := s.bucketHandler.CreateBucketIfNotExists(ctx, userInfo.InternalID); err != nil {
			log.Println(err)
			return
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// func (s *APIServer) verifyToken(cookieValue string) (bool, *userdata.VerifiedUserInfo) {
// 	resp, err := http.Get(fmt.Sprintf("https://www.googleapis.com/oauth2/v3/tokeninfo?access_token=%s", cookieValue))
// 	if err != nil {
// 		log.Println(err)
// 		return false, nil
// 	}
// 	defer resp.Body.Close()

// 	if resp.StatusCode != http.StatusOK {
// 		log.Println("Token verification failed, invalid token")
// 		return false, nil
// 	}

// 	var userInfo userdata.VerifiedUserInfo
// 	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
// 		log.Println("cannot decode user info")
// 		return false, nil
// 	}

// 	if userInfo.Email == "" || userInfo.Sub == "" {
// 		log.Println("Invalid token: Missing email or user ID")
// 		return false, nil
// 	}

// 	return true, &userInfo
// }

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

// TODO: refactor to use sessions table
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
