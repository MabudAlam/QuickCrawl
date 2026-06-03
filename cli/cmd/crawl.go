package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/core"
	"github.com/MabudAlam/quickcrawl/internal/crawler"
	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/spf13/cobra"
)

// crawlCmd represents the crawl subcommand.
// It discovers and scrapes multiple pages starting from a seed URL.
var crawlCmd = &cobra.Command{
	Use:   "crawl",
	Short: "Crawl a website and scrape multiple pages",
	Long: `Crawl a website using breadth-first search, scraping discovered pages.

The crawl starts from a seed URL and follows links up to maxDepth levels
deep, respecting robots.txt and rate limits. Results are collected and
returned when the crawl completes or maxPages is reached.

Example:
  quickcrawl crawl https://example.com
  quickcrawl crawl https://example.com --max-depth 3 --max-pages 50
  quickcrawl crawl https://example.com --formats html --render-js`,
	RunE: runCrawl,
}

var crawlFlags = struct {
	formats  string
	renderJS bool
	waitFor  int64
	maxDepth int
	maxPages int
	// renderer is deprecated and ignored.
	renderer string
}{}

func init() {
	rootCmd.AddCommand(crawlCmd)

	crawlCmd.Flags().StringVarP(&crawlFlags.formats, "formats", "f", "markdown",
		"Output formats (comma-separated): markdown,html,links")
	crawlCmd.Flags().BoolVar(&crawlFlags.renderJS, "render-js", false,
		"Force JavaScript rendering on all pages")
	crawlCmd.Flags().Int64Var(&crawlFlags.waitFor, "wait-for", 0,
		"Milliseconds to wait after page load")
	crawlCmd.Flags().IntVar(&crawlFlags.maxDepth, "max-depth", 2,
		"Maximum link depth to follow (0-10)")
	crawlCmd.Flags().IntVar(&crawlFlags.maxPages, "max-pages", 10,
		"Maximum number of pages to scrape")
	crawlCmd.Flags().StringVar(&crawlFlags.renderer, "renderer", "auto",
		"Deprecated: ignored. The scraper uses chromedp only.")
}

func runCrawl(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("a URL argument is required")
	}

	targetURL := args[0]

	parsedURL, urlErr := url.Parse(targetURL)
	if urlErr != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("invalid URL: %s", targetURL)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme: %s (only http/https)", parsedURL.Scheme)
	}

	formats := parseFormats(crawlFlags.formats)

	maxDepth := uint32(crawlFlags.maxDepth)
	if maxDepth > 10 {
		maxDepth = 10
	}
	maxPages := uint32(crawlFlags.maxPages)
	if maxPages > 1000 {
		maxPages = 1000
	}

	var renderJS *bool
	if crawlFlags.renderJS {
		renderJS = &crawlFlags.renderJS
	}

	var waitFor *int64
	if crawlFlags.waitFor > 0 {
		waitFor = &crawlFlags.waitFor
	}

	crawlReq := &types.CrawlRequest{
		URL:      targetURL,
		MaxDepth: &maxDepth,
		MaxPages: &maxPages,
		Formats:  formats,
		RenderJS: renderJS,
		WaitFor:  waitFor,
	}
	crawlReq.Defaults()

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

	jobID := fmt.Sprintf("cli-%d", time.Now().UnixNano())
	stateCh := make(chan types.CrawlState, 100)

	go func() {
		opts := crawler.CrawlOptions{
			ID:                jobID,
			Req:               crawlReq,
			Scraper:           scraper,
			MaxConcurrency:    cfg.Crawler.MaxConcurrency,
			RespectRobots:     cfg.Crawler.RespectRobotsTxt,
			RequestsPerSecond: cfg.Crawler.RequestsPerSecond,
			UserAgent:         cfg.Crawler.UserAgent,
			StateCh:           stateCh,
			JitterFactor:      cfg.Crawler.Stealth.JitterFactor,
		}
		crawler.RunCrawl(opts)
	}()

	timeout := time.Duration(cfg.Server.RequestTimeoutSecs) * time.Second
	start := time.Now()

	for {
		if time.Since(start) > timeout {
			return fmt.Errorf("crawl timed out after %v", timeout)
		}

		select {
		case state := <-stateCh:
			if state.Status == types.CrawlStatusCompleted {
				if state.Error != nil {
					return fmt.Errorf("crawl failed: %s", *state.Error)
				}
				outputCrawlResults(state)
				return nil
			}
			if state.Status == types.CrawlStatusFailed {
				errMsg := "unknown error"
				if state.Error != nil {
					errMsg = *state.Error
				}
				return fmt.Errorf("crawl failed: %s", errMsg)
			}
			if verbose {
				errPrint("crawl progress: %d/%d pages\n", state.Completed, state.Total)
			}
		case <-time.After(2 * time.Second):
			// No state update within the poll window. Loop and try again.
		}
	}
}

// outputCrawlResults formats and outputs the final crawl results.
func outputCrawlResults(state types.CrawlState) {
	for _, data := range state.Data {
		result := formatTypesScrapeData(&data)
		output("%s\n", result)
	}

	if verbose {
		errPrint("crawl completed: %d pages scraped, %d URLs discovered\n",
			state.Completed, state.Total)
	}
}

// formatTypesScrapeData formats a *types.ScrapeData (the per-page
// crawl pipeline result) as JSON.
func formatTypesScrapeData(data *types.ScrapeData) string {
	if data == nil {
		return `{"error": "no data returned"}`
	}

	result := make(map[string]interface{})
	if data.Markdown != nil && *data.Markdown != "" {
		result["markdown"] = *data.Markdown
	}
	if data.HTML != nil && *data.HTML != "" {
		result["html"] = *data.HTML
	}
	if data.PlainText != nil && *data.PlainText != "" {
		result["plainText"] = *data.PlainText
	}
	if data.Links != nil && len(data.Links) > 0 {
		result["links"] = data.Links
	}
	result["metadata"] = data.Metadata
	if data.Warning != nil && *data.Warning != "" {
		result["warning"] = *data.Warning
	}

	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to format: %v"}`, err)
	}
	return string(encoded)
}
