package crawler

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ParseSitemap parses a sitemap XML string and returns all URLs found.
func ParseSitemap(xml string) []string {
	var urls []string

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(xml))
	if err != nil {
		return parseSitemapWithRegex(xml)
	}

	// Extract URLs from <url><loc>...</loc></url> entries
	doc.Find("url > loc").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text != "" {
			urls = append(urls, text)
		}
	})

	// If no URLs found, try sitemap entries (for sitemap index files)
	if len(urls) == 0 {
		doc.Find("sitemap > loc").Each(func(i int, s *goquery.Selection) {
			text := strings.TrimSpace(s.Text())
			if text != "" {
				urls = append(urls, text)
			}
		})
	}

	if len(urls) == 0 {
		return parseSitemapWithRegex(xml)
	}

	return urls
}

// parseSitemapWithRegex uses regex to extract URLs when XML parsing fails.
func parseSitemapWithRegex(xml string) []string {
	var urls []string
	re := regexp.MustCompile(`<loc[^>]*>([^<]+)</loc>`)
	matches := re.FindAllStringSubmatch(xml, -1)
	for _, m := range matches {
		if len(m) > 1 {
			trimmed := strings.TrimSpace(m[1])
			if trimmed != "" {
				urls = append(urls, trimmed)
			}
		}
	}
	return urls
}

// FetchSitemap fetches and parses a sitemap from a URL.
func FetchSitemap(sitemapURL string, client any) []string {
	return ParseSitemapFromURL(sitemapURL, client)
}

// ParseSitemapFromURL fetches and parses a sitemap, following nested sitemaps.
func ParseSitemapFromURL(sitemapURL string, client any) []string {
	visited := map[string]struct{}{}
	return parseSitemapURLRecursive(sitemapURL, client, visited)
}

// parseSitemapURLRecursive recursively processes sitemap URLs.
// Handles sitemap index files that reference other sitemaps.
func parseSitemapURLRecursive(raw string, client any, visited map[string]struct{}) []string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	if err := isValidSitemapURL(parsed); err != nil {
		return nil
	}
	if _, ok := visited[parsed.String()]; ok {
		return nil
	}
	visited[parsed.String()] = struct{}{}

	httpClient := httpClientForSitemap(client)
	req, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "quickcrawl/0.1")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	// Limit sitemap size to 2MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil
	}

	// Parse the sitemap
	locs := ParseSitemap(string(body))
	var urls []string
	for _, loc := range locs {
		// If it's another XML file, recurse into it
		if strings.HasSuffix(strings.ToLower(strings.TrimSpace(loc)), ".xml") {
			urls = append(urls, parseSitemapURLRecursive(loc, client, visited)...)
			continue
		}
		// Otherwise, it's a page URL
		urls = append(urls, loc)
	}
	return urls
}

// httpClientForSitemap extracts or creates an HTTP client for sitemap fetching.
func httpClientForSitemap(client any) *http.Client {
	if c, ok := client.(*http.Client); ok && c != nil {
		return c
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// parseURL parses a URL string.
func parseURL(raw string) (*url.URL, error) {
	return url.Parse(raw)
}

// isValidSitemapURL validates that a sitemap URL is well-formed.
func isValidSitemapURL(u *url.URL) error {
	if u == nil {
		return fmt.Errorf("nil url")
	}
	return nil
}
