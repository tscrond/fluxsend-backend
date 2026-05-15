package main

import (
	"net/http"

	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"github.com/tscrond/fluxsend-backend/internal/config"
	"github.com/tscrond/fluxsend-backend/internal/logger"
)

type namedHTTPServer struct {
	name string
	srv  *http.Server
}

func main() {
	log := logger.New()
	defer log.Sync() //nolint:errcheck

	apiConfig := config.NewAPIServerConfig()
	cliConfig := config.NewCLIServerConfig()

	runtime, err := buildRuntime(log, apiConfig)
	if err != nil {
		log.Fatal(err)
	}
	defer runtime.Close(log)

	apiServer := buildAPIServer(log, apiConfig, runtime)
	cliServer := buildCLIServer(log, cliConfig, runtime)

	if err := runHTTPServers(log,
		namedHTTPServer{
			name: "api",
			srv: &http.Server{
				Addr:    ":" + apiConfig.ListenPort,
				Handler: apiServer.Handler(),
			},
		},
		namedHTTPServer{
			name: "cli",
			srv: &http.Server{
				Addr:    ":" + cliConfig.ListenPort,
				Handler: cliServer.Handler(),
			},
		},
	); err != nil {
		log.Fatal(err)
	}

}
