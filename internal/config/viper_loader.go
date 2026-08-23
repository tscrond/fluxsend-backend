package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// BuildViper constructs a standalone Viper instance with a single precedence chain:
// defaults < optional config file < environment variables.
func BuildViper(cfgFile, envFile string) (*viper.Viper, error) {
	v := viper.New()

	// 1. Defaults
	setDefaults(v)

	// 2. Config file
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home directory: %w", err)
		}

		v.AddConfigPath(home)
		v.SetConfigType("yaml")
		v.SetConfigName(".fluxsend-backend")
	}

	if err := v.ReadInConfig(); err != nil {
		if cfgFile != "" {
			return nil, fmt.Errorf("read config file %q: %w", cfgFile, err)
		}

		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read default config file: %w", err)
		}
	}

	// 3. .env
	if envFile != "" {
		if err := loadEnvFile(v, envFile); err != nil {
			return nil, fmt.Errorf("load env file %q: %w", envFile, err)
		}
	}

	// 4. Actual process environment
	if err := bindEnvVars(v); err != nil {
		return nil, err
	}

	for key := range GetEnvVarMap() {
		fmt.Printf(
			"[DEBUG] resolved: key=%s value=%#v isSet=%v\n",
			key,
			v.Get(key),
			v.IsSet(key),
		)
	}

	return v, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("api.listen_port", "3000")
	v.SetDefault("api.mail_from", "noreply@fluxsend.invalid")
	v.SetDefault("api.enable_google_auth", false)
	v.SetDefault("api.enable_github_auth", false)
	v.SetDefault("api.enable_password_auth", false)
	v.SetDefault("mail.provider", "standard")
	v.SetDefault("cli.listen_port", "8091")
	v.SetDefault("cli.route_prefix", "/api")
}

func bindEnvVars(v *viper.Viper) error {
	for key, envVar := range GetEnvVarMap() {
		value, exists := os.LookupEnv(envVar)
		if !exists {
			continue
		}

		fmt.Printf("[DEBUG] environment: %s=%q -> %s\n", envVar, value, key)

		if err := v.BindEnv(key, envVar); err != nil {
			return fmt.Errorf(
				"bind env %s to %s: %w",
				envVar,
				key,
				err,
			)
		}
	}

	return nil
}

func loadEnvFile(v *viper.Viper, path string) error {
	values, err := godotenv.Read(path)
	if err != nil {
		return err
	}

	envMap := GetEnvVarMap()

	for key, envVar := range envMap {
		if value, ok := values[envVar]; ok {
			fmt.Printf("[DEBUG] .env: %s=%q -> %s\n", envVar, value, key)
			v.Set(key, value)
		}
	}

	return nil
}
