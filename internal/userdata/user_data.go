package userdata

type ContextKey string

const VerifiedUserContextKey ContextKey = "verified_user"
const AuthorizedUserContextKey ContextKey = "authorized_user"

// Info gathered after successful oauth2 callback
type AuthorizedUserInfo struct {
	InternalID string `json:"internal_id"` // UUID from our database
	Id         string `json:"id"`          // OAuth subject ID
	Email      string `json:"email"`
	Name       string `json:"name"`
	GivenName  string `json:"given_name"`
	FamilyName string `json:"family_name"`
	Picture    string `json:"picture"`
	Locale     string `json:"locale"`
	Provider   string `json:"provider"`
}

// Sample google oauth2 token verification response:
//
//	{
//		"azp": "asodfkasdofkao-rasdkfkaosdfpasodfkg.apps.googleusercontent.com",
//		"aud": "asodfkasdofkao-rasdkfkaosdfpasodfkg.apps.googleusercontent.com",
//		"sub": "1065436339302349807",
//		"scope": "https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile openid",
//		"exp": "1111432532",
//		"expires_in": "4432",
//		"email": "abc.bca@gmail.com",
//		"email_verified": "true",
//		"access_type": "offline"
//	  }
type VerifiedUserInfo struct {
	Azp           string `json:"azp"`
	Aud           string `json:"aud"`
	Sub           string `json:"sub"`
	Scope         string `json:"scope"`
	Exp           string `json:"exp"`
	ExpiresIn     string `json:"expires_in"`
	Email         string `json:"email"`
	EmailVerified string `json:"email_verified"`
	AccessType    string `json:"access_type"`
}
