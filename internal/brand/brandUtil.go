package brand

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func resolveURL(src string, baseURL string) string {
	if src == "" {
		return ""
	}

	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		return src
	}

	if strings.HasPrefix(src, "//") {
		return "https:" + src
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}

	resolved := base.ResolveReference(&url.URL{Path: src})
	return resolved.String()
}

func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}

func getDomain(pageURL string) string {
	parsedURL, _ := url.Parse(pageURL)
	if parsedURL == nil {
		return ""
	}
	return strings.TrimPrefix(parsedURL.Hostname(), "www.")
}

func newDocumentFromHTML(html string) (*goquery.Document, error) {
	return goquery.NewDocumentFromReader(strings.NewReader(html))
}
