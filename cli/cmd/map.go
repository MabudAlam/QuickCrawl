package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/MabudAlam/quickcrawl/internal/config"
	"github.com/MabudAlam/quickcrawl/internal/crawler"
	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/spf13/cobra"
)

// mapCmd represents the URL discovery subcommand.
// It discovers URLs starting from a base URL without scraping content.
var mapCmd = &cobra.Command{
	Use:   "map",
	Short: "Discover URLs on a website without scraping content",
	Long: `Discover URLs starting from a base URL using breadth-first traversal.

This command only discovers URLs - it does not scrape content. It respects
robots.txt, uses rate limiting, and optionally seeds discovery from sitemaps.
Useful for building a URL list before crawling or for site audits.

Example:
  quickcrawl map https://example.com
  quickcrawl map https://example.com --max-depth 3
  quickcrawl map https://example.com --no-sitemap  # Skip sitemap discovery`,
	RunE: runMap,
}

var mapFlags = struct {
	maxDepth   int
	useSitemap bool
	timeout    int
	// renderer is deprecated and ignored.
	renderer string
}{}

func init() {
	rootCmd.AddCommand(mapCmd)

	mapCmd.Flags().IntVar(&mapFlags.maxDepth, "max-depth", 2,
		"Maximum link depth to follow (0-100, no negatives)")
	mapCmd.Flags().BoolVar(&mapFlags.useSitemap, "sitemap", true,
		"Use sitemap.xml and robots.txt sitemaps as seed URLs")
	mapCmd.Flags().IntVar(&mapFlags.timeout, "timeout", 30000,
		"Timeout in milliseconds for the entire operation (1-600000)")
	mapCmd.Flags().StringVar(&mapFlags.renderer, "renderer", "auto",
		"Deprecated: ignored. The scraper uses chromedp only.")
}

func runMap(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("a URL argument is required")
	}

	targetURL := args[0]

	maxDepth := mapFlags.maxDepth
	timeout := mapFlags.timeout

	mapReq := types.MapRequest{
		URL:        targetURL,
		MaxDepth:   &maxDepth,
		UseSitemap: &mapFlags.useSitemap,
		Timeout:    &timeout,
	}
	mapReq.Defaults()
	if err := mapReq.Validate(); err != nil {
		return err
	}

	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	scraper, qErr := config.NewScraperFromConfig(cfg, cfg.Extraction.LLM)
	if qErr != nil {
		return fmt.Errorf("failed to initialize scraper: %w", qErr)
	}
	defer scraper.Close()

	opts := crawler.MapOptions{
		BaseURL:           targetURL,
		MaxDepth:          uint32(*mapReq.MaxDepth),
		UseSitemap:        *mapReq.UseSitemap,
		Scraper:           scraper,
		MaxConcurrency:    cfg.Crawler.MaxConcurrency,
		RequestsPerSecond: cfg.Crawler.RequestsPerSecond,
		UserAgent:         cfg.Crawler.UserAgent,
		Timeout:           mapReq.Timeout,
	}

	result, mapErr := crawler.Map(opts)
	if mapErr != nil {
		return fmt.Errorf("map failed: %w", mapErr)
	}

	outputMapResults(result)

	return nil
}

func outputMapResults(result *types.MapData) {
	wrapper := map[string]interface{}{
		"links": result.Links,
		"count": len(result.Links),
	}

	encoded, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		errPrint("error formatting results: %v\n", err)
		return
	}
	output("%s\n", encoded)
}
