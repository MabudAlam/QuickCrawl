package search

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/core"
)

// Request is the unified input for the search pipeline used by the
// MCP server, HTTP API and CLI. Each transport builds this from its
// own request shape.
type Request struct {
	Query      string
	Language   string
	TimeRange  string
	Categories string
	Safesearch string
	Page       int

	// UseBM25 enables BM25F scoring against title+snippet and
	// re-ranks the results by the BM25F score.
	UseBM25 bool

	// BM25FWeights are the per-field weights used by BM25F.
	// Sourced from app config; not exposed in the request body.
	BM25FWeights BM25FWeights

	// Scrape fetches each result URL via the provided scraper.
	Scrape bool

	// Formats is the list of output formats when Scrape is true.
	Formats []string

	// RenderJS is passed to the scraper for each result URL.
	// nil = auto, true = force, false = HTTP only.
	RenderJS *bool

	// MaxWorkers controls concurrency when Scrape is true.
	// 0 means default (10).
	MaxWorkers int
}

// Result is a single normalized result in the unified response.
type Result struct {
	Position  int
	Score     float64
	BM25Score float64
	Title     string
	URL       string
	SiteName  string
	Snippet   string
	Engine    string
	Published string

	// Scraped content (populated only when Request.Scrape = true)
	Markdown  *string
	HTML      *string
	RawHTML   *string
	PlainText *string
	Links     []string
	RawJSON   []byte
}

// Response is the unified search response returned to every caller.
type Response struct {
	Query        string
	Results      []Result
	TotalResults int
	Page         int
}

// Search runs the full search pipeline:
//  1. Call SearXNG with the upstream options.
//  2. Optionally compute BM25 scores against title+snippet.
//  3. Optionally re-rank by BM25 score.
//  4. Optionally scrape each result URL in parallel.
//
// scraper may be nil if Request.Scrape is false; passing nil while
// Request.Scrape is true returns an error.
func Search(
	ctx context.Context,
	searxng *SearXNGSearcher,
	scraper *core.Scraper,
	req Request,
) (*Response, error) {
	if searxng == nil {
		return nil, fmt.Errorf("search: searxng client is not initialized")
	}
	if req.Query == "" {
		return nil, fmt.Errorf("search: query is required")
	}
	if req.Scrape && scraper == nil {
		return nil, fmt.Errorf("search: scraper is not initialized")
	}

	page := req.Page
	if page < 0 {
		page = 0
	}

	searxngResults, err := searxng.Search(ctx, Options{
		Query:      req.Query,
		Language:   req.Language,
		TimeRange:  req.TimeRange,
		Categories: req.Categories,
		Safesearch: req.Safesearch,
		Page:       page,
	})
	if err != nil {
		return nil, fmt.Errorf("search: searxng call failed: %w", err)
	}

	results := make([]Result, len(searxngResults))
	for i, r := range searxngResults {
		results[i] = Result{
			Position:  r.Position,
			Score:     r.Score,
			Title:     r.Title,
			URL:       r.URL,
			Snippet:   r.Snippet,
			Engine:    r.Engine,
			Published: r.Published,
			SiteName:  hostname(r.URL),
		}
	}

	if req.UseBM25 && len(results) > 0 {
		applyBM25F(req.Query, results, req.BM25FWeights)
		sortByBM25(results)
	}

	if req.Scrape {
		scrapeAll(ctx, scraper, results, req.Formats, req.RenderJS, req.MaxWorkers)
	}

	return &Response{
		Query:        req.Query,
		Results:      results,
		TotalResults: len(results),
		Page:         page,
	}, nil
}

// NormalizeSafesearch maps CLI/MCP tokens to SearXNG's 0/1/2 values.
func NormalizeSafesearch(s string) string {
	switch s {
	case "off":
		return "0"
	case "strict":
		return "2"
	case "moderate", "":
		return "1"
	default:
		return s
	}
}

// applyBM25F fills in BM25Score for each result using BM25F with the
// supplied per-field weights. Falls back to defaults if weights are unset.
func applyBM25F(query string, results []Result, weights BM25FWeights) {
	if weights.Title <= 0 && weights.Snippet <= 0 {
		weights = DefaultBM25FWeights()
	}
	titles := make([]string, len(results))
	snippets := make([]string, len(results))
	for i, r := range results {
		titles[i] = r.Title
		snippets[i] = r.Snippet
	}
	scores := ComputeBM25FScores(query, titles, snippets, weights)
	for i := range results {
		results[i].BM25Score = scores[i]
	}
}

// sortByBM25 orders results by BM25 score (desc) and re-assigns Position.
func sortByBM25(results []Result) {
	if len(results) <= 1 {
		return
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].BM25Score > results[j].BM25Score
	})
	for i := range results {
		results[i].Position = i + 1
	}
}

// scrapeAll fetches each result URL in parallel.
// On error the result still appears in the output, just without scraped content.
func scrapeAll(
	ctx context.Context,
	scraper *core.Scraper,
	results []Result,
	formats []string,
	renderJS *bool,
	maxWorkers int,
) {
	if maxWorkers <= 0 {
		maxWorkers = 10
	}
	if len(results) < maxWorkers {
		maxWorkers = len(results)
	}

	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	for i := range results {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			defer func() { _ = recover() }()

			sem <- struct{}{}
			defer func() { <-sem }()

			if results[idx].URL == "" {
				return
			}

			reqCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()

			data, err := scraper.Scrape(reqCtx, &core.ScrapeRequest{
				URL:      results[idx].URL,
				Formats:  formats,
				RenderJS: renderJS,
			})
			if err != nil || data == nil {
				return
			}
			results[idx].Markdown = data.Markdown
			results[idx].HTML = data.HTML
			results[idx].RawHTML = data.RawHTML
			results[idx].PlainText = data.PlainText
			results[idx].Links = data.Links
			if len(data.JSON) > 0 {
				results[idx].RawJSON = []byte(data.JSON)
			}
			if data.Metadata.SourceURL != "" {
				results[idx].URL = data.Metadata.SourceURL
			}
		}(i)
	}
	wg.Wait()
}

// hostname returns the hostname of a URL or an empty string.
func hostname(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
