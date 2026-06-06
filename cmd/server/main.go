// Package main is the entry point for the quickcrawl web scraping service.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/api"
	"github.com/MabudAlam/quickcrawl/internal/api/routes"
	"github.com/MabudAlam/quickcrawl/internal/core"
	"github.com/MabudAlam/quickcrawl/internal/utils"
)

func main() {
	utils.InitLogger()

	// Step 1: Load configuration from TOML file + environment variables.
	cfg, err := api.LoadConfig()
	if err != nil {
		utils.Log.Error(fmt.Sprintf("failed to load config: %s", err.Error()))
		os.Exit(1)
	}

	// Step 2: Build the shared *core.Scraper.
	// This is the single bootstrap that wires together the HTTP fetcher,
	// the chromedp-based renderer, and the LLM extractor — and is the
	// single render path used by /v1/scrape, /v1/crawl, /v1/map, and /v1/search.
	scraper, scrapeErr := core.NewScraperFromConfig(cfg, cfg.Extraction.LLM)
	if scrapeErr != nil {
		utils.Log.Error(fmt.Sprintf("failed to initialize core scraper: %s", scrapeErr.Message))
		os.Exit(1)
	}

	// Step 3: Initialize application state.
	// AppState holds the *core.Scraper and the crawl job tracking map.
	state, stateErr := api.NewAppState(cfg)
	if stateErr != nil {
		_ = scraper.Close()
		utils.Log.Error(fmt.Sprintf("failed to initialize server state: %s", stateErr.Message))
		os.Exit(1)
	}
	state.CoreScraper = scraper

	// Step 4: Ensure cleanup on shutdown.
	defer state.Close()

	// Step 5: Log running browser instances for debugging.
	browsers := state.RendererBrowsersInfo()
	if len(browsers) > 0 {
		for _, b := range browsers {
			utils.Log.Info("browser started", "name", b.Name, "ws_url", b.WSURL)
		}
	}

	// Step 6: Set up Gin router with routes and middleware.
	router := routes.NewRouter(state, cfg.Server.RateLimitRPS)
	engine := router.Setup()

	// Step 7: Start HTTP server with graceful shutdown support.
	port := int(cfg.Server.Port)
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, port)
	srv := &http.Server{
		Addr:    addr,
		Handler: engine,
	}

	go func() {
		utils.Log.Info(fmt.Sprintf("quickcrawl starting on %s", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			utils.Log.Error(fmt.Sprintf("server error: %s", err.Error()))
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	utils.Log.Info("shutting down server...")

	// Give outstanding requests 30 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		utils.Log.Error(fmt.Sprintf("server forced to shutdown: %s", err.Error()))
		os.Exit(1)
	}

	utils.Log.Info("server exited gracefully")
}
