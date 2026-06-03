package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/MabudAlam/quickcrawl/internal/api"
	"github.com/MabudAlam/quickcrawl/internal/browser"
	"github.com/MabudAlam/quickcrawl/internal/core"
	quickcrawl "github.com/MabudAlam/quickcrawl/internal/mcp"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	serverName    = "quickcrawl"
	serverVersion = "1.0.0"
)

func main() {
	cfg, err := api.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %s", err.Error())
	}

	// Log deprecation notice for the legacy renderer.mode field. The new
	// scraper uses chromedp only; mode is accepted for backward-compat
	// but ignored at runtime. renderer.lightpanda is no longer ignored:
	// if no Chrome WS URL is configured, MCP auto-launches LightPanda.
	if cfg.Renderer.Mode != "" && string(cfg.Renderer.Mode) != "auto" {
		log.Printf("warning: renderer.mode=%q is deprecated and ignored; the new scraper uses chromedp only", cfg.Renderer.Mode)
	}

	// If the user has not configured a Chrome WS URL, fall back to
	// auto-launching a local LightPanda. The HTTP server path does not
	// do this — it requires an explicit WS URL — but the MCP path is
	// expected to be self-contained, so we provide a default browser
	// here. The launched process is killed on shutdown.
	teardown, ensureErr := browser.EnsureRenderer(cfg)
	if ensureErr != nil {
		log.Fatalf("%s", ensureErr.Error())
	}
	if teardown != nil {
		log.Printf("LightPanda started: ws=%s", cfg.Renderer.Chrome.WSURL)
		defer func() {
			teardown()
			log.Println("LightPanda stopped")
		}()
	}

	// Build the shared *core.Scraper. This is the single render path
	// used by every MCP tool — the same code path as the HTTP API.
	scraper, scrapeErr := core.NewScraperFromConfig(cfg, cfg.Extraction.LLM)
	if scrapeErr != nil {
		log.Fatalf("failed to initialize core scraper: %s", scrapeErr.Message)
	}

	state, stateErr := api.NewAppState(cfg)
	if stateErr != nil {
		_ = scraper.Close()
		log.Fatalf("failed to initialize server state: %s", stateErr.Message)
	}
	state.CoreScraper = scraper
	defer state.Close()

	browsers := state.RendererBrowsersInfo()
	if len(browsers) > 0 {
		for _, b := range browsers {
			log.Printf("browser started: %s (%s)", b.Name, b.WSURL)
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
		log.Println("shutting down MCP server...")
		cancel()
	}()

	log.Println("MCP server starting...")
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Printf("MCP server error: %v", err)
	}

	log.Println("MCP server exited gracefully")
	os.Exit(0)
}
