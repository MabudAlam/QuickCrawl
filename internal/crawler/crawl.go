package crawler

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/common"
	"github.com/MabudAlam/quickcrawl/internal/extractor"
	"github.com/MabudAlam/quickcrawl/internal/types"
)

// RunCrawl executes a breadth-first crawl starting from a seed URL.
// It respects robots.txt, rate limits, max depth, and max pages constraints.
// The crawl runs asynchronously, sending state updates through the StateCh channel.
//
// It first validates the URL, then optionally fetches robots.txt.
// The crawler uses a queue-based BFS approach, processing URLs level by level.
// Results are collected until maxPages is reached, then returned in the final state.
func RunCrawl(opts CrawlOptions) {
	if opts.Req == nil || opts.Renderer == nil {
		emitCrawlFailure(opts.ID, opts.StateCh, "crawl options are incomplete")
		return
	}

	maxDepth := uint32(2)
	if opts.Req.MaxDepth != nil {
		maxDepth = *opts.Req.MaxDepth
		if maxDepth > 10 {
			maxDepth = 10
		}
	}

	maxPages := uint32(100)
	if opts.Req.MaxPages != nil {
		maxPages = *opts.Req.MaxPages
		if maxPages > 1000 {
			maxPages = 1000
		}
	}

	parsed, err := common.ValidateURL(opts.Req.URL)
	if err != nil || parsed == nil {
		emitCrawlFailure(opts.ID, opts.StateCh, "Only http/https URLs are allowed")
		return
	}

	origin := parsed.Scheme + "://" + parsed.Host
	if parsed.Host == "" {
		emitCrawlFailure(opts.ID, opts.StateCh, "URL has no host")
		return
	}

	robots := &RobotsTxt{}
	if opts.RespectRobots {
		robots = FetchRobotsTxt(origin, opts.UserAgent, opts.Proxy)
	}

	semaphore := make(chan struct{}, maxIntValue(opts.MaxConcurrency, 1))
	rateLimiter := newDomainRateLimiter(parsed.Host, opts.RequestsPerSecond)

	visited := map[string]struct{}{}
	queueIdx := 0
	queue := []pendingCrawlItem{{url: opts.Req.URL, depth: 0}}
	visited[normalizeURL(opts.Req.URL)] = struct{}{}
	results := make([]types.ScrapeData, 0, maxPages)

	reportProgress := func(state types.CrawlState) {
		if opts.StateCh == nil {
			return
		}
		opts.StateCh <- state
	}

	for queueIdx < len(queue) && len(results) < int(maxPages) {
		currentDepth := queue[queueIdx].depth
		var frontier []pendingCrawlItem
		for queueIdx < len(queue) && queue[queueIdx].depth == currentDepth {
			frontier = append(frontier, queue[queueIdx])
			queueIdx++
		}

		resultsCh := make(chan crawlPageResult, len(frontier))
		var wg sync.WaitGroup

		for _, item := range frontier {
			if len(results) >= int(maxPages) {
				break
			}

			parsedItem, parseErr := common.ValidateURL(item.url)
			if parseErr != nil || parsedItem == nil {
				continue
			}
			if opts.RespectRobots && !robots.IsAllowed(parsedItem.Path) {
				continue
			}

			wg.Add(1)
			semaphore <- struct{}{}
			go func(item pendingCrawlItem) {
				defer wg.Done()
				defer func() { <-semaphore }()

				sleepDur := rateLimiter.NextSleep()
				if opts.JitterFactor > 0 && sleepDur > 0 {
					sleepDur = addRandomJitter(sleepDur, opts.JitterFactor)
				}
				if sleepDur > 0 {
					time.Sleep(sleepDur)
				}

				fetchResult, fetchErr := opts.Renderer.Fetch(item.url, nil, opts.Req.RenderJS, opts.Req.WaitFor, opts.Req.Browser)
				if fetchErr != nil {
					resultsCh <- crawlPageResult{item: item, err: fetchErr}
					return
				}

				data := extractor.Extract(extractor.ExtractOptions{
					RawHTML:         fetchResult.HTML,
					RawBytes:        fetchResult.RawBytes,
					SourceURL:       fetchResult.URL,
					StatusCode:      int(fetchResult.StatusCode),
					RenderedMode:    fetchResult.RenderedWith,
					TimeTaken:       fetchResult.TimeTaken,
					Formats:         opts.Req.Formats,
					OnlyMainContent: opts.Req.OnlyMainContent,
					IncludeTags:     []string{},
					ExcludeTags:     []string{},
					CSSSelector:     nil,
					XPath:           nil,
					ChunkStrategy:   nil,
					Query:           nil,
					FilterMode:      nil,
					TopK:            nil,
				})

				// NOTE: LLM extraction is skipped per-page during crawl.
				// After crawl completes, we aggregate all markdown and call LLM once.

				var links []string
				if item.depth < maxDepth && fetchResult.HTML != "" {
					links = extractor.ExtractLinks(fetchResult.HTML, fetchResult.URL)
				}

				resultsCh <- crawlPageResult{
					item:  item,
					data:  data,
					links: links,
				}
			}(item)
		}

		go func() {
			wg.Wait()
			close(resultsCh)
		}()

		var nextQueue []pendingCrawlItem
		for res := range resultsCh {
			if res.err != nil || res.data == nil {
				continue
			}

			if len(results) < int(maxPages) {
				results = append(results, *res.data)
				reportProgress(types.CrawlState{
					ID:        opts.ID,
					Success:   true,
					Status:    types.CrawlStatusInProgress,
					Total:     uint32(len(visited)),
					Completed: uint32(len(results)),
					Data:      nil,
					Error:     nil,
				})
			}

			if res.item.depth >= maxDepth {
				continue
			}

			for _, link := range res.links {
				if len(visited) >= maxDiscoveredURLs || len(results) >= int(maxPages) {
					break
				}
				if !isSafeURL(link) {
					continue
				}
				linkParsed, linkErr := common.ValidateURL(link)
				if linkErr != nil || linkParsed == nil {
					continue
				}
				if linkParsed.Scheme+"://"+linkParsed.Host != origin {
					continue
				}
				normalized := normalizeURL(link)
				if _, ok := visited[normalized]; ok {
					continue
				}
				if opts.RespectRobots && !robots.IsAllowed(linkParsed.Path) {
					continue
				}
				visited[normalized] = struct{}{}
				nextQueue = append(nextQueue, pendingCrawlItem{url: link, depth: res.item.depth + 1})
			}
		}

		queue = append(queue, nextQueue...)
	}

	var answer json.RawMessage
	var sources []types.ChunkResult
	if includesJSONFormat(opts.Req.Formats) && opts.Req.Extract != nil && opts.LLMConfig != nil {
		answer, sources = extractAggregatedJSON(results, opts.Req.Extract, opts.Req.ChunkStrategy, opts.Req.Query, opts.Req.FilterMode, opts.Req.TopK, opts.LLMConfig)
	}

	reportProgress(types.CrawlState{
		ID:        opts.ID,
		Success:   true,
		Status:    types.CrawlStatusCompleted,
		Total:     uint32(len(visited)),
		Completed: uint32(len(results)),
		Data:      results,
		Answer:    answer,
		Sources:   sources,
		Error:     nil,
	})
}

// emitCrawlFailure sends a failure state through the channel and stops the crawl.
// Used when unrecoverable errors occur during crawl initialization.
func emitCrawlFailure(id string, stateCh chan<- types.CrawlState, errMsg string) {
	if stateCh == nil {
		return
	}
	stateCh <- types.CrawlState{
		ID:        id,
		Success:   false,
		Status:    types.CrawlStatusFailed,
		Total:     0,
		Completed: 0,
		Data:      nil,
		Error:     &errMsg,
	}
}

// DiscoverUrls performs URL discovery starting from a base URL using BFS traversal.
// It discovers URLs up to maxDepth levels deep, optionally using sitemaps as seeds.
// Returns a sorted list of unique URLs discovered (excluding the seed URL).
//
// The discovery respects robots.txt rules and uses rate limiting per domain.
// It does NOT scrape content - only collects URLs for later crawling.
// If ctx is provided and has a deadline, the operation will respect that timeout.
func DiscoverUrls(baseURL string, maxDepth uint32, useSitemap bool, renderer interface {
	Fetch(rawURL string, headers map[string]string, renderJS *bool, waitForMs *int64, browser *string) (*types.FetchResult, *types.QuickCrawlError)
}, maxConcurrency int, requestsPerSecond float64, userAgent string, proxy *string, ctx context.Context) ([]string, *types.QuickCrawlError) {
	parsed, err := common.ValidateURL(baseURL)
	if err != nil || parsed == nil {
		return nil, types.ErrInvalidRequest.New("Only http/https URLs are allowed")
	}
	if parsed.Host == "" {
		return nil, types.ErrInvalidRequest.New("URL has no host")
	}

	// Create deadline if none provided, otherwise use existing
	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	origin := parsed.Scheme + "://" + parsed.Host
	visited := map[string]struct{}{}
	queueIdx := 0
	queue := []pendingCrawlItem{{url: baseURL, depth: 0}}
	visited[normalizeURL(baseURL)] = struct{}{}

	if useSitemap {
		for _, smURL := range collectSitemapSeedURLs(origin, userAgent, proxy) {
			if ctx.Err() != nil {
				break
			}
			normalized := normalizeURL(smURL)
			if _, ok := visited[normalized]; ok {
				continue
			}
			if isSafeURL(smURL) {
				visited[normalized] = struct{}{}
				queue = append(queue, pendingCrawlItem{url: smURL, depth: 0})
			}
		}
	}

	semaphore := make(chan struct{}, maxIntValue(maxConcurrency, 1))
	rateLimiter := newDomainRateLimiter(parsed.Host, requestsPerSecond)
	discovered := map[string]struct{}{}

	for queueIdx < len(queue) && len(discovered) < maxDiscoveredURLs {
		if ctx.Err() != nil {
			break
		}

		currentDepth := queue[queueIdx].depth
		var frontier []pendingCrawlItem
		for queueIdx < len(queue) && queue[queueIdx].depth == currentDepth {
			frontier = append(frontier, queue[queueIdx])
			queueIdx++
		}

		resultsCh := make(chan []string, len(frontier))
		var wg sync.WaitGroup

		for _, item := range frontier {
			if currentDepth >= maxDepth {
				continue
			}
			if ctx.Err() != nil {
				continue
			}
			wg.Add(1)
			semaphore <- struct{}{}
			go func(item pendingCrawlItem) {
				defer wg.Done()
				defer func() { <-semaphore }()

				sleepDur := rateLimiter.NextSleep()
				if sleepDur > 0 {
					time.Sleep(sleepDur)
				}

				if ctx.Err() != nil {
					return
				}

				fetchResult, fetchErr := renderer.Fetch(item.url, map[string]string{}, newBool(false), nil, nil)
				if fetchErr != nil || fetchResult == nil {
					resultsCh <- nil
					return
				}

				resultsCh <- extractor.ExtractLinks(fetchResult.HTML, fetchResult.URL)
			}(item)
		}

		go func() {
			wg.Wait()
			close(resultsCh)
		}()

		if ctx.Err() != nil {
			break
		}

		var nextQueue []pendingCrawlItem
		for links := range resultsCh {
			for _, link := range links {
				if len(discovered) >= maxDiscoveredURLs {
					break
				}
				if !isSafeURL(link) {
					continue
				}
				linkParsed, linkErr := common.ValidateURL(link)
				if linkErr != nil || linkParsed == nil {
					continue
				}
				if linkParsed.Scheme+"://"+linkParsed.Host != origin {
					continue
				}
				normalized := normalizeURL(link)
				if _, ok := visited[normalized]; ok {
					continue
				}
				visited[normalized] = struct{}{}
				discovered[normalized] = struct{}{}
				if currentDepth+1 <= maxDepth {
					nextQueue = append(nextQueue, pendingCrawlItem{url: link, depth: currentDepth + 1})
				}
			}
		}

		queue = append(queue, nextQueue...)
	}

	result := make([]string, 0, len(discovered))
	for u := range discovered {
		result = append(result, u)
	}
	sort.Strings(result)
	return result, nil
}

// collectSitemapSeedURLs collects initial URLs from sitemaps.
// It first tries the default sitemap.xml location, then checks robots.txt
// for any sitemap declarations.
func collectSitemapSeedURLs(origin, userAgent string, proxy *string) []string {
	urls := []string{origin + "/sitemap.xml"}
	if robots := FetchRobotsTxt(origin, userAgent, proxy); robots != nil {
		urls = append(urls, robots.Sitemaps...)
	}
	return urls
}

// buildCrawlLLMInput builds the content to send to LLM for a crawl page.
// If chunking is enabled with query, uses filtered chunks.
// Otherwise, uses full markdown or truncates by MaxMarkdownChars.
func buildCrawlLLMInput(markdown string, chunkStrategy *types.ChunkStrategy, query *string, filterMode *types.FilterMode, topK *int, maxMarkdownChars *int) string {
	if markdown == "" {
		return ""
	}

	// If chunking is enabled, chunk the content and optionally filter
	if chunkStrategy != nil {
		rawChunks := extractor.ChunkText(markdown, chunkStrategy)

		if query != nil && len(*query) > 0 && len(rawChunks) > 0 {
			filtered := extractor.FilterChunksScored(rawChunks, *query, filterMode, 5)

			// Apply TopK limit
			if topK != nil && *topK < len(filtered) {
				filtered = filtered[:*topK]
			}

			// Join filtered chunks
			var sb strings.Builder
			for i, chunk := range filtered {
				if i > 0 {
					sb.WriteString("\n\n")
				}
				sb.WriteString(chunk.Content)
			}
			return sb.String()
		}

		// No query, but chunking enabled - join all chunks
		var sb strings.Builder
		for i, chunk := range rawChunks {
			if i > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(chunk)
		}
		return sb.String()
	}

	// No chunking - use MaxMarkdownChars if set
	if maxMarkdownChars != nil && *maxMarkdownChars > 0 && len(markdown) > *maxMarkdownChars {
		return markdown[:*maxMarkdownChars]
	}

	return markdown
}

// extractAggregatedJSON aggregates markdown from all crawled pages, applies
// chunking and filtering across all content, then calls LLM once to produce
// a single structured JSON answer. Returns the JSON result and the source chunks used.
func extractAggregatedJSON(results []types.ScrapeData, extract *types.ExtractOptions, chunkStrategy *types.ChunkStrategy, query *string, filterMode *types.FilterMode, topK *int, llm *types.LLMConfig) (json.RawMessage, []types.ChunkResult) {
	if len(results) == 0 {
		return nil, nil
	}

	if chunkStrategy == nil || query == nil || len(*query) == 0 {
		var sb strings.Builder
		for i, res := range results {
			if res.Markdown != nil && *res.Markdown != "" {
				if i > 0 {
					sb.WriteString("\n\n---\n\n")
				}
				sb.WriteString(*res.Markdown)
			}
		}
		combinedMarkdown := sb.String()
		if combinedMarkdown == "" {
			return nil, nil
		}

		effectiveLLM := llm
		if extract.Prompt != "" {
			effectiveLLM.ExtractionPrompt = extract.Prompt
		}
		if extract.ResponseFormat != "" {
			effectiveLLM.ResponseFormat = extract.ResponseFormat
		}

		jsonResult, err := extractStructured(combinedMarkdown, extract.Schema, effectiveLLM)
		if err != nil || jsonResult == "" {
			return nil, nil
		}
		return json.RawMessage(jsonResult), nil
	}

	var allFilteredChunks []types.ChunkResult
	for _, res := range results {
		if res.Markdown == nil || *res.Markdown == "" {
			continue
		}
		pageChunks := extractor.ChunkText(*res.Markdown, chunkStrategy)
		if len(pageChunks) == 0 {
			continue
		}
		filtered := extractor.FilterChunksScored(pageChunks, *query, filterMode, 100)
		for _, chunk := range filtered {
			chunk.URL = res.Metadata.SourceURL
			chunk.PageTitle = ""
			if res.Metadata.Title != nil {
				chunk.PageTitle = *res.Metadata.Title
			}
			allFilteredChunks = append(allFilteredChunks, chunk)
		}
	}

	if len(allFilteredChunks) == 0 {
		return nil, nil
	}

	sort.Slice(allFilteredChunks, func(i, j int) bool {
		if allFilteredChunks[i].Score == nil {
			return false
		}
		if allFilteredChunks[j].Score == nil {
			return true
		}
		return *allFilteredChunks[i].Score > *allFilteredChunks[j].Score
	})

	if topK != nil && *topK < len(allFilteredChunks) {
		allFilteredChunks = allFilteredChunks[:*topK]
	}

	var sb strings.Builder
	for i, chunk := range allFilteredChunks {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(chunk.Content)
	}
	llmInput := sb.String()

	effectiveLLM := llm
	if extract.Prompt != "" {
		effectiveLLM.ExtractionPrompt = extract.Prompt
	}
	if extract.ResponseFormat != "" {
		effectiveLLM.ResponseFormat = extract.ResponseFormat
	}

	jsonResult, err := extractStructured(llmInput, extract.Schema, effectiveLLM)
	if err != nil || jsonResult == "" {
		return nil, nil
	}

	return json.RawMessage(jsonResult), allFilteredChunks
}

// buildCrawlLLMInputWithSources builds the content to send to LLM for a crawl page.
// If chunking is enabled with query, uses filtered chunks.
// Otherwise, uses full markdown or truncates by MaxMarkdownChars.
// Returns the LLM input string and the source chunks used.
func buildCrawlLLMInputWithSources(markdown string, chunkStrategy *types.ChunkStrategy, query *string, filterMode *types.FilterMode, topK *int, maxMarkdownChars *int) (string, []types.ChunkResult) {
	if markdown == "" {
		return "", nil
	}

	if chunkStrategy != nil {
		rawChunks := extractor.ChunkText(markdown, chunkStrategy)

		if query != nil && len(*query) > 0 && len(rawChunks) > 0 {
			filtered := extractor.FilterChunksScored(rawChunks, *query, filterMode, 5)

			if topK != nil && *topK < len(filtered) {
				filtered = filtered[:*topK]
			}

			var sb strings.Builder
			for i, chunk := range filtered {
				if i > 0 {
					sb.WriteString("\n\n")
				}
				sb.WriteString(chunk.Content)
			}
			return sb.String(), filtered
		}

		var sb strings.Builder
		for i, chunk := range rawChunks {
			if i > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(chunk)
		}
		return sb.String(), nil
	}

	if maxMarkdownChars != nil && *maxMarkdownChars > 0 && len(markdown) > *maxMarkdownChars {
		return markdown[:*maxMarkdownChars], nil
	}

	return markdown, nil
}
