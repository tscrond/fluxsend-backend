package runtime

import (
	"net"
	"net/http"

	"github.com/tscrond/fluxsend-backend/internal/config"
	appmetrics "github.com/tscrond/fluxsend-backend/internal/metrics"
)

func BuildMetricsHTTPServer(metricsConfig *config.MetricsServerConfig, metrics *appmetrics.Metrics) NamedHTTPServer {
	mux := http.NewServeMux()
	mux.Handle(metricsConfig.Path, metrics.Handler())

	return NamedHTTPServer{
		Name: "metrics",
		Srv: &http.Server{
			Addr:    net.JoinHostPort(metricsConfig.BindAddress, metricsConfig.ListenPort),
			Handler: mux,
		},
	}
}
