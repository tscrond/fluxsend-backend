package config

import (
	"testing"

	"github.com/spf13/viper"
)

func TestConfigDebugLoggingEnabledReadsProcessEnvVar(t *testing.T) {
	t.Setenv(configDebugLoggingEnvVar, "true")

	if !configDebugLoggingEnabled(viper.New(), nil) {
		t.Fatal("expected debug env var to enable config debug logging")
	}
}

func TestConfigDebugLoggingEnabledReadsEnvVarFromEnvFile(t *testing.T) {
	envFileValues := map[string]string{configDebugLoggingEnvVar: "true"}

	if !configDebugLoggingEnabled(viper.New(), envFileValues) {
		t.Fatal("expected debug env var from env file to enable config debug logging")
	}
}

func TestConfigDebugLoggingEnabledFallsBackToViperValue(t *testing.T) {
	v := viper.New()
	v.Set(configDebugLoggingKey, true)

	if !configDebugLoggingEnabled(v, nil) {
		t.Fatal("expected viper config value to enable config debug logging")
	}
}
