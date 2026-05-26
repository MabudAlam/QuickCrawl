package extractor

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	httpUrlRe = regexp.MustCompile(`(?i)https?://[^\s<>"')\]]+`)
	wwwUrlRe  = regexp.MustCompile(`(?i)www\.[^\s<>"')\]]+`)
)

var nonHTTPSchemes = []string{
	"javascript:", "mailto:", "data:", "tel:", "blob:", "#",
}

func isExcludedScheme(href string) bool {
	for _, s := range nonHTTPSchemes {
		if strings.HasPrefix(href, s) {
			return true
		}
	}
	return false
}

func extractLinksFromHrefs(hrefs []string, base *url.URL, seen map[string]bool) []string {
	var links []string
	for _, href := range hrefs {
		if isExcludedScheme(href) {
			continue
		}

		absoluteURL, err := base.Parse(href)
		if err != nil {
			continue
		}

		absStr := absoluteURL.String()
		if seen[absStr] {
			continue
		}
		seen[absStr] = true
		links = append(links, absStr)
	}
	return links
}

func extractUrlsFromText(html string, base *url.URL, seen map[string]bool) []string {
	var links []string

	httpMatches := httpUrlRe.FindAllString(html, -1)
	for _, match := range httpMatches {
		if isExcludedScheme(match) {
			continue
		}
		absoluteURL, err := base.Parse(match)
		if err != nil {
			continue
		}
		absStr := absoluteURL.String()
		if seen[absStr] {
			continue
		}
		seen[absStr] = true
		links = append(links, absStr)
	}

	wwwMatches := wwwUrlRe.FindAllString(html, -1)
	for _, match := range wwwMatches {
		fullUrl := "https://" + match
		if isExcludedScheme(fullUrl) {
			continue
		}
		absoluteURL, err := base.Parse(fullUrl)
		if err != nil {
			continue
		}
		absStr := absoluteURL.String()
		if seen[absStr] {
			continue
		}
		seen[absStr] = true
		links = append(links, absStr)
	}

	return links
}

func ExtractLinks(html, baseURL string) []string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	hrefRe := regexp.MustCompile(`<a[^>]+href=["']([^"']+)["']`)
	hrefMatches := hrefRe.FindAllStringSubmatch(html, -1)

	var hrefs []string
	for _, m := range hrefMatches {
		if len(m) < 2 {
			continue
		}
		href := strings.TrimSpace(m[1])
		if href != "" {
			hrefs = append(hrefs, href)
		}
	}

	seen := make(map[string]bool)
	var allLinks []string

	allLinks = append(allLinks, extractLinksFromHrefs(hrefs, base, seen)...)
	allLinks = append(allLinks, extractUrlsFromText(html, base, seen)...)

	if len(allLinks) <= 1 {
		return allLinks
	}

	deduped := make(map[string]bool)
	var result []string
	for _, link := range allLinks {
		if deduped[link] {
			continue
		}
		deduped[link] = true
		result = append(result, link)
	}
	return result
}

func ExtractImageURLs(html, baseURL string) []string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	imgSrcRe := regexp.MustCompile(`<img[^>]+src=["']([^"']+)["']`)
	matches := imgSrcRe.FindAllStringSubmatch(html, -1)

	seen := make(map[string]bool)
	var links []string

	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		src := strings.TrimSpace(m[1])
		if src == "" || strings.HasPrefix(src, "data:") || strings.HasPrefix(src, "blob:") {
			continue
		}

		absoluteURL, err := base.Parse(src)
		if err != nil {
			continue
		}

		absStr := absoluteURL.String()
		if seen[absStr] {
			continue
		}
		seen[absStr] = true
		links = append(links, absStr)
	}

	return links
}