package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/MabudAlam/quickcrawl/internal/browser"
	"github.com/MabudAlam/quickcrawl/internal/types"
)

func loadConfigWithRenderer() (cfg *types.AppConfig, teardown func(), err error) {
	cfg, err = loadConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	teardown, ensureErr := browser.EnsureRenderer(cfg)
	if ensureErr != nil {
		return nil, nil, ensureErr
	}
	return cfg, teardown, nil
}

func setupSignalHandling() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		signal.Stop(sigCh)
		browser.StopLightPanda()
		os.Exit(128 + int(sig.(syscall.Signal)))
	}()
}

func init() {
	setupSignalHandling()
}
