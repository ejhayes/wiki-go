// Package app holds the start-up steps that the wiki-go binary and a program
// that embeds the wiki both need to run before they serve requests.
package app

import (
	"fmt"
	"path/filepath"

	"wiki-go/internal/auth"
	"wiki-go/internal/config"
	"wiki-go/internal/goldext"
	"wiki-go/internal/handlers"
	"wiki-go/internal/logger"
	"wiki-go/internal/migration"
	"wiki-go/internal/static"
)

// Init prepares the wiki for the config file at configFile and returns the
// loaded configuration. It sets package-level state, so call it once per
// process. It creates the homepage and the static assets in the wiki's root
// directory when they are missing.
func Init(configFile string) (*config.Config, error) {
	config.ConfigFilePath = configFile

	// Fix broken config file if it exists (from previous bug)
	if err := migration.FixBrokenConfig(config.ConfigFilePath); err != nil {
		logger.Warn("Failed to fix broken config: %v", err)
	}

	// Migrate user roles from old IsAdmin to new role-based system
	if err := migration.MigrateUserRoles(config.ConfigFilePath); err != nil {
		return nil, fmt.Errorf("migrating user roles: %w", err)
	}

	// Load configuration (after migration)
	cfg, err := config.LoadConfig(config.ConfigFilePath)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	// Propagate wiki timezone to the goldext shortcode renderer so that
	// :::stats recent=N::: formats edit times in the configured zone
	// (consistent with formatTime used elsewhere in templates).
	goldext.SetWikiTimezone(cfg.Wiki.Timezone)
	goldext.ConfigureContentPaths(cfg.Wiki.RootDir, cfg.Wiki.DocumentsDir)

	// Apply log level from config
	logger.Init(cfg.Wiki.LogLevel)

	// Initialize session store for persistent logins
	sessionPath := filepath.Join(cfg.Wiki.RootDir, "temp", "sessions.json")
	if err := auth.InitSessionStore(sessionPath); err != nil {
		logger.Warn("Failed to initialize session store: %v", err)
	}

	// Ensure the homepage exists
	if err := handlers.EnsureHomepageExists(cfg); err != nil {
		return nil, fmt.Errorf("creating homepage: %w", err)
	}

	// Ensure static assets exist in data directory
	if err := static.EnsureStaticAssetsExist(cfg.Wiki.RootDir); err != nil {
		return nil, fmt.Errorf("copying static assets: %w", err)
	}

	// Update handlers with config
	handlers.InitHandlers(cfg)

	return cfg, nil
}
