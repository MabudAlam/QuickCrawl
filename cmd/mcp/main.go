package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/MabudAlam/quickcrawl/internal/api"
	"github.com/MabudAlam/quickcrawl/internal/browser"
	"github.com/MabudAlam/quickcrawl/internal/config"
	quickcrawl "github.com/MabudAlam/quickcrawl/internal/mcp"
	"github.com/MabudAlam/quickcrawl/internal/utils"
	"github.com/mark3labs/mcp-go/server"
)

const (
	serverName    = "quickcrawl"
	serverVersion = "1.0.0"
)

func main() {
	utils.InitLogger()

	hooks := &server.Hooks{}

	hooks.AddOnUnregisterSession(func(ctx context.Context, session server.ClientSession) {
		utils.Log.Info("client disconnected, stopping LightPanda", "session", session.SessionID())
		browser.StopLightPanda()
	})

	cfg, err := api.LoadConfig()
	if err != nil {
		utils.Log.Error(fmt.Sprintf("failed to load config: %s", err.Error()))
		os.Exit(1)
	}

	teardown, ensureErr := browser.EnsureRenderer(cfg)
	if ensureErr != nil {
		utils.Log.Error(fmt.Sprintf("%s", ensureErr.Error()))
		os.Exit(1)
	}
	if teardown != nil {
		utils.Log.Info("LightPanda started", "ws", cfg.Renderer.Chrome.WSURL)
		defer teardown()
	}

	scraper, scrapeErr := config.NewScraperFromConfig(cfg, cfg.Extraction.LLM)
	if scrapeErr != nil {
		teardown()
		utils.Log.Error(fmt.Sprintf("failed to initialize core scraper: %s", scrapeErr.Message))
		os.Exit(1)
	}

	state, stateErr := api.NewAppState(cfg)
	if stateErr != nil {
		_ = scraper.Close()
		teardown()
		utils.Log.Error(fmt.Sprintf("failed to initialize server state: %s", stateErr.Message))
		os.Exit(1)
	}
	state.CoreScraper = scraper
	defer state.Close()
	defer teardown()

	browsers := state.RendererBrowsersInfo()
	if len(browsers) > 0 {
		for _, b := range browsers {
			utils.Log.Info("browser started", "name", b.Name, "ws_url", b.WSURL)
		}
	}

	mcpServer := server.NewMCPServer(
		serverName,
		serverVersion,
		server.WithToolCapabilities(true),
		server.WithHooks(hooks),
	)

	serverImpl := quickcrawl.NewServer(state, cfg)
	quickcrawl.AddTools(mcpServer, serverImpl)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		utils.Log.Info("shutting down MCP server...")
		browser.StopLightPanda()
		os.Exit(0)
	}()

	utils.Log.Info("MCP server starting...")
	if err := server.ServeStdio(mcpServer); err != nil {
		utils.Log.Info(fmt.Sprintf("MCP server error: %v", err))
	}

	utils.Log.Info("MCP server exited gracefully")
}
