package renderer

import (
	"testing"

	"github.com/MabudAlam/quickcrawl/internal/types"
)

type stubPageFetcher struct {
	name   string
	result *types.FetchResult
	err    *types.QuickCrawlError
}

func (s *stubPageFetcher) Fetch(url string, headers map[string]string, waitForMs *int64) (*types.FetchResult, *types.QuickCrawlError) {
	return s.result, s.err
}

func (s *stubPageFetcher) Name() string {
	return s.name
}

func (s *stubPageFetcher) SupportsJS() bool {
	return true
}

func (s *stubPageFetcher) IsAvailable() bool {
	return true
}

func TestFetchWithBrowserSkipsClientCrashAndFallsBackToNextBrowser(t *testing.T) {
	httpRendered := "http"
	renderer := &FallbackRenderer{
		jsRenderers: []PageFetcher{
			&stubPageFetcher{
				name: "lightpanda",
				result: &types.FetchResult{
					URL:          "https://www.notion.com/",
					StatusCode:   200,
					HTML:         "<html><body><main><h2>Application error: a client-side exception has occurred</h2></main></body></html>",
					RenderedWith: &httpRendered,
				},
			},
			&stubPageFetcher{
				name: "chrome",
				result: &types.FetchResult{
					URL:        "https://www.notion.com/",
					StatusCode: 200,
					HTML: "<html><body><main><h1>Meet the night shift.</h1>" +
						"<p>Notion agents keep work moving 24/7 with enough content to be a good result.</p></main></body></html>",
				},
			},
		},
	}

	httpResult := &types.FetchResult{
		URL:        "https://www.notion.com/",
		StatusCode: 200,
		HTML:       "<html><body><div id=\"__next\"></div></body></html>",
	}

	got, err := renderer.fetchWithBrowser("https://www.notion.com/", nil, nil, httpResult, nil)
	if err != nil {
		t.Fatalf("fetchWithBrowser returned error: %v", err)
	}
	if got == nil {
		t.Fatal("expected browser result, got nil")
	}
	if got.RenderedWith != nil && *got.RenderedWith == "lightpanda" {
		t.Fatalf("expected broken lightpanda result to be skipped")
	}
	if !contains(got.HTML, "Meet the night shift.") {
		t.Fatalf("expected chrome content to win, got: %s", got.HTML)
	}
}

func TestFetchWithBrowserPrefersRicherHTTPOverThinBrowser(t *testing.T) {
	renderedWith := "http"
	renderer := &FallbackRenderer{
		jsRenderers: []PageFetcher{
			&stubPageFetcher{
				name: "chrome",
				result: &types.FetchResult{
					URL:          "https://example.com/",
					StatusCode:   200,
					HTML:         "<html><body><main><a href=\"/product/wikis\">Try it</a><h2>Knowledge Base</h2></main></body></html>",
					RenderedWith: &renderedWith,
				},
			},
		},
	}

	httpResult := &types.FetchResult{
		URL:        "https://example.com/",
		StatusCode: 200,
		HTML: "<html><body><main><h1>Meet the night shift.</h1><p>Notion agents keep work moving 24/7. " +
			"They capture knowledge, answer questions, and push projects forward while you sleep.</p></main></body></html>",
	}

	got, err := renderer.fetchWithBrowser("https://example.com/", nil, nil, httpResult, nil)
	if err != nil {
		t.Fatalf("fetchWithBrowser returned error: %v", err)
	}
	if got != httpResult {
		t.Fatalf("expected richer HTTP result to be returned")
	}
	if got.Warning == nil || *got.Warning == "" {
		t.Fatal("expected warning when falling back to richer HTTP result")
	}
}

func TestDetectClientSideCrash(t *testing.T) {
	html := "<html><body><main><h2>Application error: a client-side exception has occurred</h2></main></body></html>"
	if !detectClientSideCrash(html) {
		t.Fatal("expected client-side crash marker to be detected")
	}
}

func TestFetchWithPinnedRendererUnavailableReturnsError(t *testing.T) {
	renderer := &FallbackRenderer{}
	httpResult := &types.FetchResult{
		URL:        "https://example.com/",
		StatusCode: 200,
		HTML:       "<html><body><main>fallback</main></body></html>",
	}
	preferred := "chrome"

	_, err := renderer.fetchWithBrowser("https://example.com/", nil, nil, httpResult, &preferred)
	if err == nil {
		t.Fatal("expected error for unavailable pinned renderer")
	}
}

func TestFetchWithPinnedRendererFailureDoesNotFallBackToHTTP(t *testing.T) {
	preferred := "chrome"
	renderer := &FallbackRenderer{
		jsRenderers: []PageFetcher{
			&stubPageFetcher{
				name: "chrome",
				err:  types.ErrRendererError.New("cdp failed"),
			},
		},
	}
	httpResult := &types.FetchResult{
		URL:        "https://example.com/",
		StatusCode: 200,
		HTML:       "<html><body><main>http content</main></body></html>",
	}

	_, err := renderer.fetchWithBrowser("https://example.com/", nil, nil, httpResult, &preferred)
	if err == nil {
		t.Fatal("expected pinned renderer failure to return an error")
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && stringContains(s, sub))
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
