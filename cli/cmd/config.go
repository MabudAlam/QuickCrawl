package cmd

import (
	"github.com/MabudAlam/quickcrawl/internal/config"
	"github.com/MabudAlam/quickcrawl/internal/types"
)

// configLoadAppConfig is a thin wrapper around config.LoadAppConfig that
// lives in this package so subcommand files don't need to import
// internal/config directly. Kept as a separate function for testability.
func configLoadAppConfig() (*types.AppConfig, error) {
	return config.LoadAppConfig()
}
