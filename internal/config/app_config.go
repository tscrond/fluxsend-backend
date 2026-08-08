package config

import "github.com/spf13/viper"

type AppConfig struct {
	Env string
}

func NewAppConfig(v *viper.Viper) *AppConfig {
	env := v.GetString("app.env")
	if env == "" {
		env = "development"
	}

	return &AppConfig{Env: env}
}
