package config

import "github.com/spf13/viper"

type AdminServerConfig struct {
	Enabled    bool
	ListenPort string

	AdminUsername string
	AdminPassword string
}

func NewAdminServerConfig(v *viper.Viper) (*AdminServerConfig, error) {
	adminUsername := v.GetString("admin.admin_username")
	adminPassword := v.GetString("admin.admin_password")
	listenPort := v.GetString("admin.listen_port")
	if listenPort == "" {
		listenPort = "1414"
	}

	enabled := v.GetBool("admin.enabled")
	if !enabled || adminUsername == "" || adminPassword == "" {
		return &AdminServerConfig{Enabled: false, ListenPort: listenPort}, nil
	}

	return &AdminServerConfig{
		Enabled:       true,
		ListenPort:    listenPort,
		AdminUsername: adminUsername,
		AdminPassword: adminPassword,
	}, nil
}
