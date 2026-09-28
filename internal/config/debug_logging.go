package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

const (
	configDebugLoggingKey    = "app.debug_config_loading"
	configDebugLoggingEnvVar = "DEBUG_CONFIG_LOADING"
)

func configDebugLoggingEnabled(v *viper.Viper, envFileValues map[string]string) bool {
	if value, ok := os.LookupEnv(configDebugLoggingEnvVar); ok {
		return parseDebugBool(value)
	}

	if value, ok := envFileValues[configDebugLoggingEnvVar]; ok {
		return parseDebugBool(value)
	}

	if v == nil {
		return false
	}

	return v.GetBool(configDebugLoggingKey)
}

func parseDebugBool(value string) bool {
	enabled, err := strconv.ParseBool(strings.TrimSpace(value))
	return err == nil && enabled
}
