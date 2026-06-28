package crawler

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// urlset and sitemapindex match the sitemap.org 0.9 schema. Both carry
// <loc>...</loc> entries; <sitemap> wraps a nested sitemap in an index file.
type sitemapURLSet struct {
	XMLName xml.Name    `xml:"urlset"`
	URLs    []sitemapURL `xml:"url"`
	Sitemaps []sitemapURL `xml:"sitemap"`
}

type sitemapURL struct {
	Loc string `xml:"loc"`
}

// sitemapIndex wraps <sitemapindex><sitemap><loc></loc></sitemap>...</sitemapindex>.
type sitemapIndex struct {
	XMLName   xml.Name      `xml:"sitemapindex"`
	Sitemaps  []sitemapURL `xml:"sitemap"`
}

// ParseSitemap parses a sitemap XML string and returns all URLs found.
// Handles both <urlset> (regular sitemap) and <sitemapindex> (index of sitemaps).
func ParseSitemap(xmlStr string) []string {
	var urls []string

	if doc := tryParse[sitemapURLSet](xmlStr); doc != nil {
		for _, u := range doc.URLs {
			if u.Loc != "" {
				urls = append(urls, strings.TrimSpace(u.Loc))
			}
		}
		for _, s := range doc.Sitemaps {
			if s.Loc != "" {
				urls = append(urls, strings.TrimSpace(s.Loc))
			}
		}
		return urls
	}

	if doc := tryParse[sitemapIndex](xmlStr); doc != nil {
		for _, s := range doc.Sitemaps {
			if s.Loc != "" {
				urls = append(urls, strings.TrimSpace(s.Loc))
			}
		}
		return urls
	}

	return urls
}

func tryParse[T any](xmlStr string) *T {
	var doc T
	dec := xml.NewDecoder(strings.NewReader(xmlStr))
	dec.Strict = false
	if err := dec.Decode(&doc); err != nil {
		return nil
	}
	return &doc
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
