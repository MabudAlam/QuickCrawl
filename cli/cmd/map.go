package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/MabudAlam/quickcrawl/internal/crawler"
	"github.com/MabudAlam/quickcrawl/internal/renderer"
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

// mapFlags holds the configuration for the map command.
var mapFlags = struct {
	maxDepth   int
	useSitemap bool
	timeout    int
	renderer   string
	proxy      string
}{}

func init() {
	rootCmd.AddCommand(mapCmd)

	mapCmd.Flags().IntVar(&mapFlags.maxDepth, "max-depth", 2,
		"Maximum link depth to follow (0-10)")
	mapCmd.Flags().BoolVar(&mapFlags.useSitemap, "sitemap", true,
		"Use sitemap.xml and robots.txt sitemaps as seed URLs")
	mapCmd.Flags().IntVar(&mapFlags.timeout, "timeout", 30000,
		"Timeout in milliseconds for the entire operation")
	mapCmd.Flags().StringVar(&mapFlags.renderer, "renderer", "auto",
		"Renderer: auto, lightpanda, chrome")
	mapCmd.Flags().StringVar(&mapFlags.proxy, "proxy", "",
		"Proxy URL for requests")
}

// runMap executes the URL discovery command.
func runMap(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("a URL argument is required")
	}

	targetURL := args[0]

	// Validate URL.
	parsedURL, urlErr := url.Parse(targetURL)
	if urlErr != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("invalid URL: %s", targetURL)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme: %s (only http/https)", parsedURL.Scheme)
	}

	// Load configuration.
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create renderer.
	rend, rendErr := renderer.NewFallbackRendererWithConfig(
		&cfg.Renderer,
		cfg.Crawler.UserAgent,
		cfg.Crawler.Proxy,
		&cfg.Crawler.Stealth,
		cfg.Renderer.RenderJSDefault,
	)
	if rendErr != nil {
		return fmt.Errorf("failed to initialize renderer: %w", rendErr)
	}
	defer rend.Close()

	// Set up options.
	maxDepth := uint32(mapFlags.maxDepth)
	if maxDepth > 10 {
		maxDepth = 10
	}

	timeout := mapFlags.timeout
	if timeout <= 0 {
		timeout = 30000
	}

	var proxy *string
	if mapFlags.proxy != "" {
		proxy = &mapFlags.proxy
	}

	opts := crawler.MapOptions{
		BaseURL:           targetURL,
		MaxDepth:          maxDepth,
		UseSitemap:        mapFlags.useSitemap,
		Renderer:          rend,
		MaxConcurrency:    cfg.Crawler.MaxConcurrency,
		RequestsPerSecond: cfg.Crawler.RequestsPerSecond,
		UserAgent:         cfg.Crawler.UserAgent,
		Proxy:             proxy,
		Timeout:           &timeout,
	}

	// Discover URLs.
	result, mapErr := crawler.Map(opts)
	if mapErr != nil {
		return fmt.Errorf("map failed: %w", mapErr)
	}

	// Output results - one JSON object with array of links.
	outputMapResults(result)

	return nil
}

// outputMapResults formats and outputs the discovered URLs.
func outputMapResults(result *types.MapData) {
	// Wrap in a JSON object for easy parsing.
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