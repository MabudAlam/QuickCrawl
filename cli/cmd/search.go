package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/MabudAlam/quickcrawl/internal/config"
	"github.com/MabudAlam/quickcrawl/internal/core"
	"github.com/MabudAlam/quickcrawl/internal/search"
	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/MabudAlam/quickcrawl/internal/utils"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search the web and optionally scrape results",
	Long: `Search SearXNG and optionally scrape content from results.

This command searches SearXNG, then for each result optionally fetches
the content using the in-process chromedp-based scraper. Results include
title, URL, snippet, and scraped content in requested formats.

Note: Scraping individual results requires a separate fetch, so results
are processed concurrently with a default of 10 workers.

Example:
  quickcrawl search "golang web scraping"
  quickcrawl search "golang" --formats markdown --region us-en
  quickcrawl search "golang" --scrape --formats html
  quickcrawl search "golang" --use-bm25`,
	RunE: runSearch,
}

var searchFlags = struct {
	formats    string
	region     string
	safesearch string
	timelimit  string
	renderMode string
	scrape     bool
	useBM25    bool
	workers    int
	renderer   string
}{}

func init() {
	rootCmd.AddCommand(searchCmd)

	searchCmd.Flags().StringVar(&searchFlags.formats, "formats", "markdown",
		"Output formats (comma-separated): markdown,html,links,json")
	searchCmd.Flags().StringVar(&searchFlags.region, "region", "us-en",
		"Region code for search results (e.g., us-en, gb-en)")
	searchCmd.Flags().StringVar(&searchFlags.safesearch, "safesearch", "moderate",
		"SafeSearch mode: moderate, strict, off")
	searchCmd.Flags().StringVar(&searchFlags.timelimit, "timelimit", "",
		"Time limit filter (d=day, w=week, m=month, y=year)")
	searchCmd.Flags().StringVar(&searchFlags.renderMode, "render-mode", "auto",
		"Render mode for scraping each result: auto, browser, http")
	searchCmd.Flags().BoolVar(&searchFlags.scrape, "scrape", false,
		"Also scrape content from each result URL")
	searchCmd.Flags().BoolVar(&searchFlags.useBM25, "use-bm25", false,
		"Re-rank results using BM25 algorithm (default: false)")
	searchCmd.Flags().IntVar(&searchFlags.workers, "workers", 10,
		"Number of concurrent workers for scraping results")
	searchCmd.Flags().StringVar(&searchFlags.renderer, "renderer", "auto",
		"Deprecated: ignored. The scraper uses chromedp only.")
}

func runSearch(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("a search query is required")
	}
	query := args[0]
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("search query cannot be empty")
	}

	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if err := cfg.Search.Validate(); err != nil {
		return fmt.Errorf("search configuration error: %w", err)
	}

	if searchFlags.renderer != "" && searchFlags.renderer != "auto" {
		utils.Log.Warn("--renderer is deprecated and ignored; the new scraper uses chromedp only", "value", searchFlags.renderer)
	}

	searxng := search.NewSearXNG(cfg.Search.BaseURL, cfg.Search.TimeoutSecs)

	var scraper *core.Scraper
	if searchFlags.scrape {
		scraperCfg, teardown, err := loadConfigWithRenderer()
		if err != nil {
			return err
		}
		if teardown != nil {
			defer teardown()
		}
		s, err := config.NewScraperFromConfig(scraperCfg, scraperCfg.Extraction.LLM)
		if err != nil {
			return fmt.Errorf("failed to initialize scraper: %w", err)
		}
		defer s.Close()
		scraper = s
	}

	formats := parseFormats(searchFlags.formats)
	formatStrs := formatsToStrings(formats)
	if searchFlags.scrape && len(formatStrs) == 0 {
		formatStrs = []string{"markdown"}
	}

	mode, modeErr := types.ParseRenderMode(searchFlags.renderMode)
	if modeErr != nil {
		return fmt.Errorf("invalid --render-mode: %w", modeErr)
	}
	var renderModePtr *types.RenderMode
	if mode != "" {
		m := mode
		renderModePtr = &m
	}

	resp, err := search.Search(context.Background(), searxng, scraper, search.Request{
		Query:      query,
		Language:   searchFlags.region,
		TimeRange:  searchFlags.timelimit,
		Safesearch: search.NormalizeSafesearch(searchFlags.safesearch),
		UseBM25:    searchFlags.useBM25,
		BM25FWeights: search.BM25FWeights{
			Title:   cfg.Search.BM25FTitleWeight,
			Snippet: cfg.Search.BM25FSnippetWeight,
		},
		Scrape:     searchFlags.scrape,
		Formats:    formatStrs,
		RenderMode: renderModePtr,
		MaxWorkers: searchFlags.workers,
	})
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	return outputCLIResponse(resp)
}

// outputCLIResponse prints the unified search response in the CLI's
// existing JSON wire format (query, results, total_results, page).
func outputCLIResponse(resp *search.Response) error {
	type resultEntry struct {
		Position  int     `json:"position"`
		Score     float64 `json:"score"`
		BM25Score float64 `json:"bm25_score,omitempty"`
		SiteName  string  `json:"site_name"`
		Snippet   string  `json:"snippet"`
		Title     string  `json:"title"`
		URL       string  `json:"url"`
	}

	results := make([]resultEntry, len(resp.Results))
	for i, it := range resp.Results {
		results[i] = resultEntry{
			Position:  it.Position,
			Score:     it.Score,
			BM25Score: it.BM25Score,
			SiteName:  it.SiteName,
			Snippet:   it.Snippet,
			Title:     it.Title,
			URL:       it.URL,
		}
	}

	wrapper := map[string]interface{}{
		"query":         resp.Query,
		"results":       results,
		"total_results": resp.TotalResults,
		"page":          resp.Page,
	}

	encoded, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format results: %w", err)
	}
	output("%s\n", encoded)
	return nil
}
