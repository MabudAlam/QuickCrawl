package crawler

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/temoto/robotstxt"
)

var robotsClient = &http.Client{
	Timeout: 10 * time.Second,
}

type RobotsTxt struct {
	data      *robotstxt.RobotsData
	Sitemaps  []string
	userAgent string
}

func ParseRobotsTxt(text string, userAgent string) *RobotsTxt {
	rd, err := robotstxt.FromString(text)
	if err != nil {
		return &RobotsTxt{userAgent: userAgent}
	}
	return &RobotsTxt{
		data:      rd,
		Sitemaps:  rd.Sitemaps,
		userAgent: userAgent,
	}
}

func (r *RobotsTxt) IsAllowed(path string) bool {
	if r.data == nil {
		return true
	}

	if r.userAgent == "" {
		return r.data.TestAgent(path, "*")
	}

	specificAllowed := r.data.TestAgent(path, r.userAgent)
	globalAllowed := r.data.TestAgent(path, "*")

	if !globalAllowed {
		return specificAllowed
	}

	if !specificAllowed {
		return false
	}

	return true
}

func FetchRobotsTxt(origin, userAgent string) *RobotsTxt {
	robotsURL := strings.TrimRight(origin, "/") + "/robots.txt"
	u, err := url.Parse(robotsURL)
	if err != nil {
		return &RobotsTxt{userAgent: userAgent}
	}

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return &RobotsTxt{userAgent: userAgent}
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	resp, err := robotsClient.Do(req)
	if err != nil {
		return &RobotsTxt{userAgent: userAgent}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return &RobotsTxt{userAgent: userAgent}
	}

	return ParseRobotsTxt(string(body), userAgent)
}

func CheckRobotsTxt(rawURL, userAgent string) *types.QuickCrawlError {
	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL == nil {
		return nil
	}

	origin := parsedURL.Scheme + "://" + parsedURL.Host
	log.Printf("[robots] checking robots.txt for origin=%s with userAgent=%q", origin, userAgent)

	robots := FetchRobotsTxt(origin, userAgent)
	if robots == nil {
		return nil
	}

	if !robots.IsAllowed(parsedURL.Path) {
		log.Printf("[robots] denied: path=%s", parsedURL.Path)
		return &types.QuickCrawlError{
			Message: "access denied by robots.txt",
			Code:    types.CodeForbidden,
		}
	}

	log.Printf("[robots] allowed: path=%s", parsedURL.Path)
	return nil
}
