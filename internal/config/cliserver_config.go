package config

import (
	"os"

	"github.com/tscrond/fluxsend-backend/pkg"
)

type CLIServerConfig struct {
	ListenPort      string
	BackendEndpoint string
	RoutePrefix     string
}

func NewCLIServerConfig() *CLIServerConfig {
	cliConfig := CLIServerConfig{
		ListenPort:      os.Getenv("FLUXSEND_CLI_LISTEN_PORT"),
		BackendEndpoint: pkg.ReadConfigRequired("BACKEND_ENDPOINT"),
		RoutePrefix:     os.Getenv("FLUXSEND_CLI_ROUTE_PREFIX"),
	}
	if cliConfig.ListenPort == "" {
		cliConfig.ListenPort = os.Getenv("FLUXSEND_API_LISTEN_PORT")
	}
	if cliConfig.ListenPort == "" {
		cliConfig.ListenPort = "3001"
	}
	if cliConfig.RoutePrefix == "" {
		cliConfig.RoutePrefix = "/cli"
	}

	return &cliConfig
}
