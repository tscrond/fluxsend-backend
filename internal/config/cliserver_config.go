package config

import "github.com/spf13/viper"

type CLIServerConfig struct {
	ListenPort      string
	BackendEndpoint string
	RoutePrefix     string
}

func NewCLIServerConfig(v *viper.Viper) (*CLIServerConfig, error) {
	backendEndpoint, err := requiredString(v, "cli.backend_endpoint")
	if err != nil {
		return nil, err
	}

	cliConfig := CLIServerConfig{
		ListenPort:      v.GetString("cli.listen_port"),
		BackendEndpoint: backendEndpoint,
		RoutePrefix:     v.GetString("cli.route_prefix"),
	}

	return &cliConfig, nil
}
