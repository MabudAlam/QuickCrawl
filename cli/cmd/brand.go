package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/brand"
	"github.com/MabudAlam/quickcrawl/internal/config"
	"github.com/MabudAlam/quickcrawl/internal/core"
	"github.com/MabudAlam/quickcrawl/internal/crawler"
	"github.com/spf13/cobra"
)

var brandCmd = &cobra.Command{
	Use:   "brand",
	Short: "Extract brand identity data from a website",
	Long: `Extract comprehensive brand design tokens and metadata from a website.

This command fetches a URL, renders it in a browser (if available), and
extracts brand identity signals: colors, fonts, logos, favicons, social links,
open-graph metadata, and styleguide tokens.

Example:
  quickcrawl brand https://example.com`,
	RunE: runBrand,
}

func init() {
	rootCmd.AddCommand(brandCmd)
}

func runBrand(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("a URL argument is required")
	}

	targetURL := args[0]

	cfg, teardown, err := loadConfigWithRenderer()
	if err != nil {
		return err
	}
	if teardown != nil {
		defer teardown()
	}

	parsedURL, _ := url.Parse(targetURL)
	if parsedURL != nil {
		origin := parsedURL.Scheme + "://" + parsedURL.Host
		if cfg.Crawler.RespectRobotsTxt {
			robots := crawler.FetchRobotsTxt(origin, cfg.Crawler.UserAgent)
			if robots != nil && !robots.IsAllowed(parsedURL.Path) {
				return fmt.Errorf("access denied by robots.txt")
			}
		}
	}

	scraper, qErr := config.NewScraperFromConfig(cfg, cfg.Extraction.LLM)
	if qErr != nil {
		return fmt.Errorf("failed to initialize scraper: %w", qErr)
	}
	defer scraper.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	result, fetchErr := scraper.FetchBrand(ctx, targetURL)
	if fetchErr != nil {
		return handleBrandError(fetchErr)
	}

	brandData := brand.ExtractMetadataWithTokens(result.HTML, targetURL, result.Tokens)

	domain := extractDomain(targetURL)
	brandData.Domain = domain

	encoded, err := json.MarshalIndent(brandData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal brand data: %w", err)
	}

	output("%s\n", string(encoded))
	return nil
}

func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	return host
}

func handleBrandError(err *core.QuickCrawlError) error {
	if err == nil {
		return nil
	}

	code := err.Code
	msg := err.Message

	switch code {
	case core.CodeRendererError:
		return fmt.Errorf("render error: %s", msg)
	case core.CodeTimeout:
		return fmt.Errorf("request timed out: %s", msg)
	case core.CodeForbidden:
		if msg != "" {
			return fmt.Errorf("access forbidden: %s", msg)
		}
		return fmt.Errorf("access denied by robots.txt")
	case core.CodeRateLimited:
		return fmt.Errorf("rate limited: %s", msg)
	case core.CodeHttp:
		return fmt.Errorf("HTTP error: %s", msg)
	case core.CodeInvalidURL:
		return fmt.Errorf("invalid URL: %s", msg)
	default:
		return fmt.Errorf("brand extraction failed: %s", msg)
	}
}
