package config

type AuthConfig struct {
	GoogleOAuthConfig GoogleOAuthConfig
}

type GoogleOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
	Endpoint     string
}

// 	ClientID:      clientId,
// 	ClientSecret:  clientSecret,
// 	RedirectURL:   fmt.Sprintf("%s/auth/callback", backendEndpoint),
// 	Scopes:        []string{"email", "profile"},
// 	Endpoint:      google.Endpoint,
// 	AuthProviders: authProviders,
// }
