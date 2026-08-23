/*
Copyright © 2026 Tomasz Skrond <>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package cmd

import (
	"log"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"github.com/tscrond/fluxsend-backend/internal/config"
	"github.com/tscrond/fluxsend-backend/internal/logger"
	runtime "github.com/tscrond/fluxsend-backend/internal/runtime"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "fluxsend",
	Short: "FluxSend Backend",
	Run: func(cmd *cobra.Command, args []string) {
		generateConfig, _ := cmd.Flags().GetString("generate-config")
		log.Println("generateConfig:", generateConfig)

		if generateConfig, _ := cmd.Flags().GetString("generate-config"); generateConfig != "" {
			RunConfigGenerator(cmd, args)
			return
		}

		if devAPI, _ := cmd.Flags().GetBool("dev-api"); devAPI {
			RunDeveloperAPI(cmd, args)
		} else {
			RunFullBackend(cmd, args)
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.fluxsend-backend.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("dev-api", "d", false, "Run only developer API")
	rootCmd.Flags().BoolP("github-auth", "g", false, "Enable GitHub OAuth authentication")
	rootCmd.Flags().BoolP("google-auth", "o", false, "Enable Google OAuth authentication")
	rootCmd.Flags().BoolP("password-auth", "p", false, "Enable password authentication")
	rootCmd.Flags().String("env-file", "", "Path to .env file to load environment variables from")
	rootCmd.Flags().String("generate-config", "$HOME/.fluxsend-backend.yaml", "Generate default config file in specified location and exit (if env file is specified - use it for generation)")
}

func RunDeveloperAPI(cmd *cobra.Command, args []string) {
	envFile, _ := cmd.Flags().GetString("env-file")

	v, err := config.BuildViper(cfgFile, envFile)
	if err != nil {
		cobra.CheckErr(err)
	}

	appConfig := config.NewAppConfig(v)
	log := logger.New(appConfig.Env)
	defer log.Sync() //nolint:errcheck

	baseRuntimeConfig, err := config.NewBaseRuntimeConfig(v)
	if err != nil {
		log.Fatal(err)
	}

	cliConfig, err := config.NewCLIServerConfig(v)
	if err != nil {
		log.Fatal(err)
	}

	baseRuntime, err := runtime.BuildBaseRuntime(log, baseRuntimeConfig)
	if err != nil {
		log.Fatal(err)
	}
	defer baseRuntime.Close(log)

	cliServer := runtime.BuildCLIServer(log, cliConfig, baseRuntime)

	if err := runtime.RunHTTPServers(log,
		runtime.NamedHTTPServer{
			Name: "cli",
			Srv: &http.Server{
				Addr:    ":" + cliConfig.ListenPort,
				Handler: cliServer.Handler(),
			},
		},
	); err != nil {
		log.Fatal(err)
	}
}

func RunFullBackend(cmd *cobra.Command, args []string) {
	envFile, _ := cmd.Flags().GetString("env-file")

	v, err := config.BuildViper(cfgFile, envFile)
	if err != nil {
		cobra.CheckErr(err)
	}

	applyAuthFlagOverrides(cmd, v)

	appConfig := config.NewAppConfig(v)
	log := logger.New(appConfig.Env)
	defer log.Sync() //nolint:errcheck

	baseRuntimeConfig, err := config.NewBaseRuntimeConfig(v)
	if err != nil {
		log.Fatal(err)
	}

	apiConfig, err := config.NewAPIServerConfig(v)
	if err != nil {
		log.Fatal(err)
	}
	cliConfig, err := config.NewCLIServerConfig(v)
	if err != nil {
		log.Fatal(err)
	}

	baseRuntime, err := runtime.BuildBaseRuntime(log, baseRuntimeConfig)
	if err != nil {
		log.Fatal(err)
	}
	defer baseRuntime.Close(log)

	apiServerRuntime, err := runtime.BuildAPIServerRuntime(log, apiConfig, baseRuntime)
	if err != nil {
		log.Fatal(err)
	}

	apiServer := runtime.BuildAPIServer(log, apiConfig, baseRuntime, apiServerRuntime)
	cliServer := runtime.BuildCLIServer(log, cliConfig, baseRuntime)

	if err := runtime.RunHTTPServers(log,
		runtime.NamedHTTPServer{
			Name: "api",
			Srv: &http.Server{
				Addr:    ":" + apiConfig.ListenPort,
				Handler: apiServer.Handler(),
			},
		},
		runtime.NamedHTTPServer{
			Name: "cli",
			Srv: &http.Server{
				Addr:    ":" + cliConfig.ListenPort,
				Handler: cliServer.Handler(),
			},
		},
	); err != nil {
		log.Fatal(err)
	}

}

func RunConfigGenerator(cmd *cobra.Command, args []string) {
	generateConfig, _ := cmd.Flags().GetString("generate-config")
	envFile, _ := cmd.Flags().GetString("env-file")
	v, err := config.BuildViper(cfgFile, envFile)
	if err != nil {
		cobra.CheckErr(err)
	}

	if err := config.GenerateDefaultConfig(v, generateConfig); err != nil {
		cobra.CheckErr(err)
	}
}

func applyAuthFlagOverrides(cmd *cobra.Command, v configSetter) {
	authFlagMappings := []struct {
		flagName  string
		configKey string
	}{
		{flagName: "github-auth", configKey: "api.enable_github_auth"},
		{flagName: "google-auth", configKey: "api.enable_google_auth"},
		{flagName: "password-auth", configKey: "api.enable_password_auth"},
	}

	for _, mapping := range authFlagMappings {
		flag := cmd.Flags().Lookup(mapping.flagName)
		if flag == nil || !flag.Changed {
			continue
		}

		value, err := cmd.Flags().GetBool(mapping.flagName)
		if err != nil {
			cobra.CheckErr(err)
		}

		v.Set(mapping.configKey, value)
	}
}

type configSetter interface {
	Set(key string, value any)
}
