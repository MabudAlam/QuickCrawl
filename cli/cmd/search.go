package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/MabudAlam/quickcrawl/internal/crawler"
	"github.com/MabudAlam/quickcrawl/internal/renderer"
	"github.com/MabudAlam/quickcrawl/internal/search"
	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/MabudAlam/quickcrawl/internal/utils"
	"github.com/spf13/cobra"
)

// searchCmd represents the web search subcommand.
// It searches DuckDuckGo and optionally scrapes each result.
var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search the web and optionally scrape results",
	Long: `Search DuckDuckGo and optionally scrape content from results.

This command searches DuckDuckGo, then for each result optionally fetches
the content using the configured renderer. Results include title, URL,
snippet, and scraped content in requested formats.

Note: Scraping individual results requires a separate fetch, so results
are processed concurrently with a default of 10 workers.

Example:
  quickcrawl search "golang web scraping"
  quickcrawl search "golang" --formats markdown --region us-en
  quickcrawl search "golang" --scrape --formats html`,
	RunE: runSearch,
}

// searchFlags holds the configuration for the search command.
var searchFlags = struct {
	formats    string
	region     string
	safesearch string
	timelimit  string
	renderJS   bool
	scrape     bool
	workers    int
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
	searchCmd.Flags().BoolVar(&searchFlags.renderJS, "render-js", false,
		"Render JavaScript when scraping result pages")
	searchCmd.Flags().BoolVar(&searchFlags.scrape, "scrape", false,
		"Also scrape content from each result URL")
	searchCmd.Flags().IntVar(&searchFlags.workers, "workers", 10,
		"Number of concurrent workers for scraping results")
}

// runSearch executes the search command.
func runSearch(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("a search query is required")
	}

	query := args[0]

	// Validate query isn't empty after trimming.
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("search query cannot be empty")
	}

	// Set defaults from flags or use sensible defaults.
	region := searchFlags.region
	if region == "" {
		region = "us-en"
	}

	safesearch := searchFlags.safesearch
	if safesearch == "" {
		safesearch = "moderate"
	}

	// Perform the search.
	engine := search.New()
	results, searchErr := engine.Search(query, region, safesearch, searchFlags.timelimit)
	if searchErr != nil {
		return fmt.Errorf("search failed: %w", searchErr)
	}

	if len(results) == 0 {
		output(`{"results": [], "count": 0}`+"\n")
		return nil
	}

	// If --scrape flag is set, fetch content from each result.
	if searchFlags.scrape {
		return scrapeSearchResults(results)
	}

	// Just output the search results without scraping.
	return outputSearchResults(results)
}

// scrapeSearchResults fetches content from each search result URL.
// Results are processed concurrently with a configurable number of workers.
func scrapeSearchResults(results []search.TextResult) error {
	// Load configuration.
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create renderer.
	rend, rendErr := renderer.NewFallbackRendererWithConfig(
		&cfg.Renderer,
		cfg.Crawler.UserAgent,
		&cfg.Crawler.Stealth,
		cfg.Renderer.RenderJSDefault,
	)
	if rendErr != nil {
		return fmt.Errorf("failed to initialize renderer: %w", rendErr)
	}
	defer rend.Close()

	// Parse formats.
	formats := parseFormats(searchFlags.formats)

	// Determine number of workers.
	// Use the smaller of configured workers or result count.
	maxWorkers := searchFlags.workers
	if maxWorkers <= 0 {
		maxWorkers = 10
	}
	if len(results) < maxWorkers {
		maxWorkers = len(results)
	}

	// Semaphore to limit concurrent requests.
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Map to store results in original order.
	resultMap := make(map[int]types.SearchResult, len(results))

	for i, r := range results {
		wg.Add(1)
		sem <- struct{}{}

		go func(index int, result search.TextResult) {
			defer wg.Done()
			defer func() { <-sem }()

			// Default to just the search result data (title, URL, snippet).
			searchResult := types.SearchResult{
				Title:       result.Title,
				Description: result.Body,
				URL:         result.Href,
			}

			// Validate URL before scraping.
			if _, urlErr := url.Parse(result.Href); urlErr == nil {
				renderJS := searchFlags.renderJS
				scrapeReq := &types.ScrapeRequest{
					URL:      result.Href,
					Formats:  formats,
					RenderJS: &renderJS,
				}

				data, scrapeErr := crawler.ScrapeURL(
					scrapeReq,
					rend,
					cfg.Extraction.LLM,
					cfg.Crawler.Stealth.Enabled,
					cfg.Renderer.RenderJSDefault,
					utils.HeaderStrategy(cfg.Crawler.Stealth.Strategy),
				)

				if scrapeErr == nil && data != nil {
					if data.Markdown != nil {
						searchResult.Markdown = data.Markdown
					}
					if data.HTML != nil {
						searchResult.HTML = data.HTML
					}
					if data.PlainText != nil {
						searchResult.PlainText = data.PlainText
					}
					if data.Links != nil {
						searchResult.Links = data.Links
					}
					if len(data.JSON) > 0 {
						searchResult.RawJSON = data.JSON
					}
					if data.Metadata.SourceURL != "" {
						searchResult.URL = data.Metadata.SourceURL
					}
				}
			}

			mu.Lock()
			resultMap[index] = searchResult
			mu.Unlock()
		}(i, r)
	}

	wg.Wait()

	// Collect results in order.
	orderedResults := make([]types.SearchResult, 0, len(results))
	for i := range results {
		orderedResults = append(orderedResults, resultMap[i])
	}

	return outputSearchScrapedResults(orderedResults)
}

// outputSearchResults outputs raw search results without scraping.
func outputSearchResults(results []search.TextResult) error {
	// Convert to the standard search result format.
	searchResults := make([]types.SearchResult, len(results))
	for i, r := range results {
		searchResults[i] = types.SearchResult{
			Title:       r.Title,
			Description: r.Body,
			URL:         r.Href,
		}
	}

	wrapper := map[string]interface{}{
		"results": searchResults,
		"count":   len(searchResults),
	}

	encoded, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format results: %w", err)
	}

	output("%s\n", encoded)
	return nil
}

// outputSearchScrapedResults outputs search results with scraped content.
func outputSearchScrapedResults(results []types.SearchResult) error {
	wrapper := map[string]interface{}{
		"results": results,
		"count":   len(results),
	}

	encoded, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format results: %w", err)
	}

	output("%s\n", encoded)
	return nil
}