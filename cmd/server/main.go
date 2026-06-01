// Package main is the entry point for the quickcrawl web scraping service.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/api"
	"github.com/MabudAlam/quickcrawl/internal/api/routes"
	"github.com/MabudAlam/quickcrawl/internal/core"
	"github.com/MabudAlam/quickcrawl/internal/renderer"
	"github.com/MabudAlam/quickcrawl/internal/utils"
)

// main initializes and starts the quickcrawl HTTP server.
// It loads configuration, sets up the renderer and handlers,
// then listens for incoming requests.
func main() {
	// Step 1: Load configuration from TOML file + environment variables.
	// Config includes server port, crawler settings, renderer mode, etc.
	cfg, err := api.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %s", err.Error())
	}

	// Step 2: Initialize application state.
	// AppState holds crawl job tracking (in-memory map protected by mutex),
	// and manages job expiration via a background goroutine.
	state, stateErr := api.NewAppState(cfg)
	if stateErr != nil {
		log.Fatalf("failed to initialize server state: %s", stateErr.Message)
	}

	// Step 3: Initialize the page renderer.
	// FallbackRenderer orchestrates multiple fetch strategies:
	// - HTTP fetcher (always available, fastest)
	// - Browser-based fetchers (Chrome, LightPanda) for JS-heavy pages
	// It uses a fallback pattern: HTTP first, then escalates to browser rendering
	// when needed (SPA detection, anti-bot challenges, auth blocks).
	rend, rendererErr := renderer.NewFallbackRendererWithConfig(
		&cfg.Renderer,
		cfg.Crawler.UserAgent,
		&cfg.Crawler.Stealth,
		cfg.Renderer.RenderJSDefault,
	)
	if rendererErr != nil {
		log.Fatalf("failed to initialize renderer: %s", rendererErr.Message)
	}

	state.Renderer = rend

	// Step 3b: Build a shared *renderer.HTTPFetcher so both endpoints use the
	// same HTTP code path (no duplication). The FallbackRenderer keeps its own
	// internal instance because its API is fixed.
	var coreStealthProfile *utils.HeaderProfile
	if cfg.Crawler.Stealth.Enabled && cfg.Crawler.Stealth.InjectHeaders {
		profile := utils.GetHeaderProfile(utils.HeaderStrategy(cfg.Crawler.Stealth.Strategy))
		coreStealthProfile = &profile
	}
	coreHTTPFetcher := renderer.NewHTTPFetcher(cfg.Crawler.UserAgent, coreStealthProfile)

	// Step 3c: Initialize the core scraper (chromedp-based) with the shared HTTP fetcher.
	coreCfg := core.DefaultConfig()
	if cfg.Renderer.Chrome != nil && strings.TrimSpace(cfg.Renderer.Chrome.WSURL) != "" {
		coreCfg.Browser.WSURL = strings.TrimSpace(cfg.Renderer.Chrome.WSURL)
	}
	if cfg.Renderer.PoolSize > 0 {
		coreCfg.Browser.PoolSize = cfg.Renderer.PoolSize
	}
	coreScraper, coreErr := core.NewScraper(coreCfg, coreHTTPFetcher, cfg.Extraction.LLM)
	if coreErr != nil {
		log.Fatalf("failed to initialize core scraper: %s", coreErr.Message)
	}
	state.CoreScraper = coreScraper

	// Step 4: Ensure cleanup on shutdown.
	defer state.Close()

	// Step 5: Log running browser instances for debugging.
	browsers := rend.BrowsersInfo()
	if len(browsers) > 0 {
		for _, b := range browsers {
			log.Printf("browser started: %s (%s)", b.Name, b.WSURL)
		}
	}

	// Step 6: Set up Gin router with routes and middleware.
	// Routes:
	//   GET  /health           - Health check with renderer/browser status
	//   POST /v1/scrape        - Scrape a single URL
	//   POST /v1/crawl         - Start an async BFS crawl job
	//   GET  /v1/crawl/:id     - Get crawl job status/results
	//   DELETE /v1/crawl/:id   - Cancel a crawl job
	//   POST /v1/map           - Discover URLs without scraping content
	// Middleware: CORS, optional rate limiting
	router := routes.NewRouter(state, cfg.Server.RateLimitRPS)
	engine := router.Setup()

	// Step 7: Start HTTP server with graceful shutdown support.
	port := int(cfg.Server.Port)
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, port)
	srv := &http.Server{
		Addr:    addr,
		Handler: engine,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("quickcrawl starting on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %s", err.Error())
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down server...")

	// Give outstanding requests 30 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server forced to shutdown: %s", err.Error())
	}

	log.Println("server exited gracefully")
}
