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

// @title           FluxSend Developer API
// @version         1.7.0
// @description     Fluxsend API endpoints

// @contact.name   tscrond
// @contact.url    https://github.com/tscrond

// @license.name  MIT
// @license.url   https://mit-license.org/

// @host      fluxsend.win
// @BasePath  /

// @securityDefinitions.apikey ApiKeyAuth
// @name X-API-Key
// @in header

// @externalDocs.description  OpenAPI
// @externalDocs.url          https://swagger.io/resources/open-api/
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
