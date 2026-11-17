package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"wiki-go/internal/app"
	"wiki-go/internal/config"
	"wiki-go/internal/logger"
	"wiki-go/internal/routes"
)

func main() {
	configfilepath := flag.String("configfile",
		GetEnvString("CONFIGFILE", config.ConfigFilePath),
		"where to find config.yaml")
	flag.Parse()

	// Run the start-up steps that an embedding program also runs
	cfg, err := app.Init(*configfilepath)
	if err != nil {
		logger.Fatal("Error %v", err)
	}

	// Setup all routes
	routes.SetupRoutes(cfg)

	// Start the server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	if cfg.Server.SSL && cfg.Server.SSLCert != "" && cfg.Server.SSLKey != "" {
		logger.Info("HTTPS server starting on %s...", addr)
		if err := http.ListenAndServeTLS(addr, cfg.Server.SSLCert, cfg.Server.SSLKey, nil); err != nil {
			logger.Fatal("HTTPS server error: %v", err)
		}
	} else {
		logger.Info("HTTP server starting on %s...", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			logger.Fatal("HTTP server error: %v", err)
		}
	}
}

func GetEnvString(name, defaultvalue string) string {
	value, ok := os.LookupEnv(name)
	if !ok {
		return defaultvalue
	}
	return value
}
