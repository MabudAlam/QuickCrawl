package crawler

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/common"
	"github.com/MabudAlam/quickcrawl/internal/core"
	"github.com/MabudAlam/quickcrawl/internal/extractor"
	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/MabudAlam/quickcrawl/internal/utils"
)

// RunCrawl executes a breadth-first crawl starting from a seed URL.
// It respects robots.txt, rate limits, max depth, and max pages constraints.
// The crawl runs asynchronously, sending state updates through the StateCh channel.
//
// It first validates the URL, then optionally fetches robots.txt.
// The crawler uses a queue-based BFS approach, processing URLs level by level.
// Results are collected until maxPages is reached, then returned in the final state.
//
// The Scraper field on opts is the shared *core.Scraper used for every page
// fetch. It provides both the HTTP and chromedp-based browser paths so
// crawl pages use the exact same code as the /v1/scrape endpoint.
func RunCrawl(opts CrawlOptions) {
	if opts.Req == nil || opts.Scraper == nil {
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
		robots = FetchRobotsTxt(origin, opts.UserAgent)
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

				var headers map[string]string
				if opts.StealthStrategy != "" {
					profile := utils.GetHeaderProfile(opts.StealthStrategy)
					headers = profile.ToMap()
				}

				renderJS := opts.Req.RenderJS != nil && *opts.Req.RenderJS
				waitMs := int64(0)
				if opts.Req.WaitFor != nil {
					waitMs = *opts.Req.WaitFor
				}

				fetchCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				fetchResult, fetchErr := opts.Scraper.FetchHTML(fetchCtx, item.url, headers, renderJS, waitMs, opts.Req.Browser)
				cancel()
				if fetchErr != nil {
					resultsCh <- crawlPageResult{item: item, err: convertCoreCrawlError(fetchErr)}
					return
				}

				data := extractor.Extract(extractor.ExtractOptions{
					RawHTML:        fetchResult.HTML,
					RawBytes:       fetchResult.RawBytes,
					SourceURL:      fetchResult.URL,
					StatusCode:     int(fetchResult.StatusCode),
					RenderedMode:   fetchResult.RenderedWith,
					TimeTaken:      fetchResult.TimeTaken,
					Formats:        opts.Req.Formats,
					IncludeTags:    []string{},
					ExcludeTags:    []string{},
					CSSSelector:    nil,
				})

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

	reportProgress(types.CrawlState{
		ID:        opts.ID,
		Success:   true,
		Status:    types.CrawlStatusCompleted,
		Total:     uint32(len(visited)),
		Completed: uint32(len(results)),
		Data:      results,
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

// convertCoreCrawlError maps a *core.QuickCrawlError into the
// *types.QuickCrawlError used by the rest of the crawl pipeline. The two
// types are structurally identical but live in different packages so
// the rest of the pipeline keeps a single error type.
func convertCoreCrawlError(e *core.QuickCrawlError) *types.QuickCrawlError {
	if e == nil {
		return nil
	}
	return &types.QuickCrawlError{
		Message: e.Message,
		Code:    types.ErrorCode(string(e.Code)),
	}
}

// DiscoverUrls performs URL discovery starting from a base URL using BFS traversal.
// It discovers URLs up to maxDepth levels deep, optionally using sitemaps as seeds.
// Returns a sorted list of unique URLs discovered (excluding the seed URL).
//
// The discovery respects robots.txt rules and uses rate limiting per domain.
// It does NOT scrape content - only collects URLs for later crawling.
// If ctx is provided and has a deadline, the operation will respect that timeout.
func DiscoverUrls(baseURL string, maxDepth uint32, useSitemap bool, scraper *core.Scraper, respectRobots bool, maxConcurrency int, requestsPerSecond float64, userAgent string, ctx context.Context) ([]string, *types.QuickCrawlError) {
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

	// Fetch robots.txt if respectRobots is enabled
	var robots *RobotsTxt
	if respectRobots {
		robots = FetchRobotsTxt(origin, userAgent)
		// Check if the base URL itself is allowed
		if robots != nil && !robots.IsAllowed(parsed.Path) {
			return nil, types.ErrForbidden.New("access denied by robots.txt")
		}
	}

	if useSitemap {
		for _, smURL := range collectSitemapSeedURLs(origin, userAgent) {
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

				// Check robots.txt before fetching
				if respectRobots && robots != nil {
					parsedLink, parseErr := common.ValidateURL(item.url)
					if parseErr == nil && !robots.IsAllowed(parsedLink.Path) {
						resultsCh <- nil
						return
					}
				}

				fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				fetchResult, fetchErr := scraper.FetchHTML(fetchCtx, item.url, map[string]string{}, false, 0, nil)
				cancel()
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
func collectSitemapSeedURLs(origin, userAgent string) []string {
	urls := []string{origin + "/sitemap.xml"}
	if robots := FetchRobotsTxt(origin, userAgent); robots != nil {
		urls = append(urls, robots.Sitemaps...)
	}
	return urls
}
