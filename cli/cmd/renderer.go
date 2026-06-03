package cmd

import (
	"fmt"
	"log"

	"github.com/MabudAlam/quickcrawl/internal/browser"
	"github.com/MabudAlam/quickcrawl/internal/types"
)

// loadConfigWithRenderer loads the application config and, when no
// Chrome WS URL has been configured, auto-launches a local LightPanda
// and patches cfg.Renderer.Chrome so the scraper can use it.
//
// The returned teardown function must be called on shutdown to reap
// the spawned LightPanda process. It is a no-op when no browser was
// launched (e.g. the user already configured ws_url, or the command
// does not need a browser).
//
// This helper is the CLI counterpart of the same logic in
// cmd/mcp/main.go. The HTTP server does NOT use it: in the server
// deployment model the user is expected to supply
// [renderer.chrome].ws_url explicitly.
func loadConfigWithRenderer() (cfg *types.AppConfig, teardown func(), err error) {
	cfg, err = loadConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	teardown, ensureErr := browser.EnsureRenderer(cfg)
	if ensureErr != nil {
		return nil, nil, ensureErr
	}
	if teardown != nil {
		log.Printf("LightPanda auto-started: ws=%s", cfg.Renderer.Chrome.WSURL)
	}
	return cfg, teardown, nil
}
