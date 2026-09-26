package config

import "github.com/microcosm-cc/bluemonday"

type BackendConfig struct {
	ListenPort             string
	FrontendEndpoint       string
	BackendEndpoint        string
	MailFrom               string
	HTMLSanitizationPolicy *bluemonday.Policy
	AuthWhitelist          []string
	AdminUsername          string
	AdminPassword          string
}
