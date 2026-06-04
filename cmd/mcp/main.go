package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/MabudAlam/quickcrawl/internal/api"
	"github.com/MabudAlam/quickcrawl/internal/browser"
	"github.com/MabudAlam/quickcrawl/internal/core"
	quickcrawl "github.com/MabudAlam/quickcrawl/internal/mcp"
	"github.com/MabudAlam/quickcrawl/internal/utils"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	serverName    = "quickcrawl"
	serverVersion = "1.0.0"
)

func main() {
	utils.InitLogger()

	cfg, err := api.LoadConfig()
	if err != nil {
		utils.Log.Error(fmt.Sprintf("failed to load config: %s", err.Error()))
		os.Exit(1)
	}

	// If the user has not configured a Chrome WS URL, fall back to
	// auto-launching a local LightPanda. The HTTP server path does not
	// do this — it requires an explicit WS URL — but the MCP path is
	// expected to be self-contained, so we provide a default browser
	// here. The launched process is killed on shutdown.
	teardown, ensureErr := browser.EnsureRenderer(cfg)
	if ensureErr != nil {
		utils.Log.Error(fmt.Sprintf("%s", ensureErr.Error()))
		os.Exit(1)
	}
	if teardown != nil {
		utils.Log.Info("LightPanda started", "ws", cfg.Renderer.Chrome.WSURL)
		defer func() {
			teardown()
			utils.Log.Info("LightPanda stopped")
		}()
	}

	// Build the shared *core.Scraper. This is the single render path
	// used by every MCP tool — the same code path as the HTTP API.
	scraper, scrapeErr := core.NewScraperFromConfig(cfg, cfg.Extraction.LLM)
	if scrapeErr != nil {
		utils.Log.Error(fmt.Sprintf("failed to initialize core scraper: %s", scrapeErr.Message))
		os.Exit(1)
	}

	state, stateErr := api.NewAppState(cfg)
	if stateErr != nil {
		_ = scraper.Close()
		utils.Log.Error(fmt.Sprintf("failed to initialize server state: %s", stateErr.Message))
		os.Exit(1)
	}
	state.CoreScraper = scraper
	defer state.Close()

	browsers := state.RendererBrowsersInfo()
	if len(browsers) > 0 {
		for _, b := range browsers {
			utils.Log.Info("browser started", "name", b.Name, "ws_url", b.WSURL)
		}
	}

	server := mcp.NewServer(
		&mcp.Implementation{Name: serverName, Version: serverVersion},
		nil,
	)

	serverImpl := quickcrawl.NewServer(state, cfg)
	quickcrawl.AddTools(server, serverImpl)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		utils.Log.Info("shutting down MCP server...")
		cancel()
	}()

	utils.Log.Info("MCP server starting...")
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		utils.Log.Info(fmt.Sprintf("MCP server error: %v", err))
	}

	utils.Log.Info("MCP server exited gracefully")
	os.Exit(0)
}
