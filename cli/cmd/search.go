package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/core"
	"github.com/MabudAlam/quickcrawl/internal/search"
	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/spf13/cobra"
)

// searchCmd represents the web search subcommand.
// It searches DuckDuckGo and optionally scrapes each result.
var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search the web and optionally scrape results",
	Long: `Search DuckDuckGo and optionally scrape content from results.

This command searches DuckDuckGo, then for each result optionally fetches
the content using the in-process chromedp-based scraper. Results include
title, URL, snippet, and scraped content in requested formats.

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
	// renderer is deprecated and ignored.
	renderer string
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
	searchCmd.Flags().StringVar(&searchFlags.renderer, "renderer", "auto",
		"Deprecated: ignored. The scraper uses chromedp only.")
}

// runSearch executes the search command.
func runSearch(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("a search query is required")
	}

	query := args[0]

	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("search query cannot be empty")
	}

	region := searchFlags.region
	if region == "" {
		region = "us-en"
	}

	safesearch := searchFlags.safesearch
	if safesearch == "" {
		safesearch = "moderate"
	}

	// Log deprecation notice if the user pinned a renderer backend.
	if searchFlags.renderer != "" && searchFlags.renderer != "auto" {
		log.Printf("[cli.search] warning: --renderer is deprecated and ignored; the new scraper uses chromedp only (value=%q)", searchFlags.renderer)
	}

	engine := search.New()
	results, searchErr := engine.Search(query, region, safesearch, searchFlags.timelimit)
	if searchErr != nil {
		return fmt.Errorf("search failed: %w", searchErr)
	}

	if len(results) == 0 {
		output(`{"results": [], "count": 0}` + "\n")
		return nil
	}

	if searchFlags.scrape {
		return scrapeSearchResults(results)
	}

	return outputSearchResults(results)
}

// scrapeSearchResults fetches content from each search result URL using
// the shared *core.Scraper (chromedp + HTTP). Results are processed
// concurrently with a configurable number of workers.
func scrapeSearchResults(results []search.TextResult) error {
	cfg, teardown, err := loadConfigWithRenderer()
	if err != nil {
		return err
	}
	if teardown != nil {
		defer teardown()
	}

	scraper, qErr := core.NewScraperFromConfig(cfg, cfg.Extraction.LLM)
	if qErr != nil {
		return fmt.Errorf("failed to initialize scraper: %w", qErr)
	}
	defer scraper.Close()

	formats := parseFormats(searchFlags.formats)

	maxWorkers := searchFlags.workers
	if maxWorkers <= 0 {
		maxWorkers = 10
	}
	if len(results) < maxWorkers {
		maxWorkers = len(results)
	}

	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex

	resultMap := make(map[int]types.SearchResult, len(results))

	for i, r := range results {
		wg.Add(1)
		go func(index int, result search.TextResult) {
			defer wg.Done()
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("search: panic recovered while scraping %s: %v", result.Href, rec)
					mu.Lock()
					resultMap[index] = types.SearchResult{
						Title:       result.Title,
						Description: result.Body,
						URL:         result.Href,
					}
					mu.Unlock()
				}
			}()

			sem <- struct{}{}
			defer func() { <-sem }()

			searchResult := types.SearchResult{
				Title:       result.Title,
				Description: result.Body,
				URL:         result.Href,
			}

			if _, urlErr := url.Parse(result.Href); urlErr != nil {
				log.Printf("search: skipping invalid URL %s: %v", result.Href, urlErr)
				mu.Lock()
				resultMap[index] = searchResult
				mu.Unlock()
				return
			}

			renderJS := searchFlags.renderJS
			scrapeReq := &core.ScrapeRequest{
				URL:      result.Href,
				Formats:  formatsToStrings(formats),
				RenderJS: &renderJS,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			data, scrapeErr := scraper.Scrape(ctx, scrapeReq)
			cancel()

			if scrapeErr != nil {
				log.Printf("search: failed to scrape %s: %v", result.Href, scrapeErr)
			} else if data != nil {
				if data.Markdown != nil {
					s := *data.Markdown
					searchResult.Markdown = &s
				}
				if data.HTML != nil {
					s := *data.HTML
					searchResult.HTML = &s
				}
				if data.RawHTML != nil {
					s := *data.RawHTML
					searchResult.RawHTML = &s
				}
				if data.PlainText != nil {
					s := *data.PlainText
					searchResult.PlainText = &s
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

			mu.Lock()
			resultMap[index] = searchResult
			mu.Unlock()
		}(i, r)
	}

	wg.Wait()

	orderedResults := make([]types.SearchResult, 0, len(results))
	for i := range results {
		orderedResults = append(orderedResults, resultMap[i])
	}

	return outputSearchScrapedResults(orderedResults)
}

// outputSearchResults outputs raw search results without scraping.
func outputSearchResults(results []search.TextResult) error {
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
