package config

import "github.com/spf13/viper"

type MetricsServerConfig struct {
	Enabled     bool
	ListenPort  string
	BindAddress string
	Path        string
}

func NewMetricsServerConfig(v *viper.Viper) (*MetricsServerConfig, error) {
	listenPort := v.GetString("metrics.listen_port")
	if listenPort == "" {
		listenPort = "9464"
	}

	path := v.GetString("metrics.path")
	if path == "" {
		path = "/metrics"
	}

	return &MetricsServerConfig{
		Enabled:     v.GetBool("metrics.enabled"),
		ListenPort:  listenPort,
		BindAddress: v.GetString("metrics.bind_address"),
		Path:        path,
	}, nil
}
