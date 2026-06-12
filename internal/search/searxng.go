package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SearXNGResult is a single normalized result returned by a SearXNG instance.
// It includes the position in the result set, the title, URL, snippet,
// engine, and (optionally) a published date string.
type SearXNGResult struct {
	Position   int
	Title      string
	URL        string
	Snippet    string
	Engine     string
	Published  string
	Score      float64 // Relevance score 0-100 derived from position (100 = most relevant)
}

// SearXNGSearcher queries a SearXNG /search?format=json endpoint and
// returns the results in the order SearXNG emits them (1-based position).
type SearXNGSearcher struct {
	BaseURL string
	HTTP    *http.Client
}

// NewSearXNG creates a SearXNGSearcher with the given base URL (no trailing
// slash) and timeout. A 0 timeout falls back to 30s.
func NewSearXNG(baseURL string, timeoutSecs int) *SearXNGSearcher {
	if timeoutSecs <= 0 {
		timeoutSecs = 30
	}
	return &SearXNGSearcher{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP: &http.Client{
			Timeout: time.Duration(timeoutSecs) * time.Second,
		},
	}
}

// Options controls a single SearXNG query.
type Options struct {
	Query      string
	Language   string // "auto", "en", "all", etc.
	TimeRange  string // "day", "month", "year", or "" for any.
	Categories string // "general", "news", "general,images", etc.
	Safesearch string // "0", "1", "2".
	Page       int    // 1-based page number.
}

// Search performs a GET against <baseURL>/search with the configured
// options and returns the parsed results. Returns an error if the upstream
// call fails or the response body cannot be parsed.
func (s *SearXNGSearcher) Search(ctx context.Context, opts Options) ([]SearXNGResult, error) {
	if s == nil {
		return nil, fmt.Errorf("searxng: nil searcher")
	}
	if strings.TrimSpace(opts.Query) == "" {
		return nil, fmt.Errorf("searxng: query is required")
	}
	if opts.Page <= 0 {
		opts.Page = 1
	}

	params := url.Values{}
	params.Set("q", opts.Query)
	params.Set("format", "json")
	if opts.Language != "" {
		params.Set("language", opts.Language)
	}
	if opts.TimeRange != "" {
		params.Set("time_range", opts.TimeRange)
	}
	if opts.Categories != "" {
		params.Set("categories", opts.Categories)
	}
	if opts.Safesearch != "" {
		params.Set("safesearch", opts.Safesearch)
	}
	params.Set("pageno", fmt.Sprintf("%d", opts.Page))

	endpoint := s.BaseURL + "/search?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("searxng: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")

	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searxng: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		// Truncate body for log friendliness.
		if len(body) > 512 {
			body = body[:512]
		}
		return nil, fmt.Errorf("searxng: upstream status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("searxng: read body: %w", err)
	}

	var raw searxngResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("searxng: parse body: %w", err)
	}

	results := make([]SearXNGResult, 0, len(raw.Results))
	for _, r := range raw.Results {
		results = append(results, SearXNGResult{
			Title:     strings.TrimSpace(r.Title),
			URL:       r.URL,
			Snippet:   strings.TrimSpace(r.Content),
			Engine:    r.Engine,
			Published: coalescePublished(r),
			Score:     r.Score,
		})
	}
	// Assign 1-based positions (score comes from SearXNG).
	for i := range results {
		results[i].Position = i + 1
	}
	return results, nil
}

// searxngResponse mirrors the JSON shape returned by SearXNG with
// format=json. Only the fields we need are decoded.
type searxngResponse struct {
	Query    string         `json:"query"`
	Results  []searxngEntry `json:"results"`
	Answers  []any          `json:"answers"`
	Infoboxes []any         `json:"infoboxes"`
}

type searxngEntry struct {
	URL           string  `json:"url"`
	Title         string  `json:"title"`
	Content       string  `json:"content"`
	Engine        string  `json:"engine"`
	PublishedDate string  `json:"publishedDate"`
	PubDate       string  `json:"pubdate"`
	Category      string  `json:"category"`
	Score         float64 `json:"score"`
}

// coalescePublished picks the first non-empty publish date from a
// SearXNG result entry, since different engines use different fields.
func coalescePublished(r searxngEntry) string {
	if r.PublishedDate != "" {
		return r.PublishedDate
	}
	return r.PubDate
}
