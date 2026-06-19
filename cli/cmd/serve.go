package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/MabudAlam/quickcrawl/internal/browser"
	"github.com/MabudAlam/quickcrawl/internal/config"
	"github.com/MabudAlam/quickcrawl/internal/utils"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start QuickCrawl with LightPanda browser (session-based lifecycle)",
	Long: `Start QuickCrawl CLI with a persistent LightPanda browser instance.

LightPanda is started when this command runs and stopped when the
process receives SIGINT/SIGTERM or when the command is interrupted.

This is useful for running multiple CLI commands while keeping the
browser warm, similar to how the MCP server manages the browser lifecycle.

Examples:
  quickcrawl serve
  quickcrawl serve --port 8080`,
	RunE: runServe,
}

var serveFlags struct {
	port int
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().IntVarP(&serveFlags.port, "port", "p", 8080, "Port to run the serve command (placeholder for future HTTP API)")
}

func runServe(cmd *cobra.Command, args []string) error {
	utils.InitLogger()

	cfg, err := config.LoadAppConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	teardown, ensureErr := browser.EnsureRenderer(cfg)
	if ensureErr != nil {
		return fmt.Errorf("failed to start browser: %w", ensureErr)
	}
	if teardown != nil {
		defer teardown()
	}

	scraper, scrapeErr := config.NewScraperFromConfig(cfg, cfg.Extraction.LLM)
	if scrapeErr != nil {
		return fmt.Errorf("failed to initialize scraper: %w", scrapeErr)
	}
	defer scraper.Close()

	browsers := scraper.BrowsersInfo()
	if len(browsers) > 0 {
		for _, b := range browsers {
			utils.Log.Info("browser started", "name", b.Name, "ws_url", b.WSURL)
		}
	}

	utils.Log.Info("QuickCrawl serve started with LightPanda")
	utils.Log.Info("Press Ctrl+C to stop")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	utils.Log.Info("shutting down serve...")
	browser.StopLightPanda()
	return nil
}