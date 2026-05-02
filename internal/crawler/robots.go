package crawler

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var robotsClient = &http.Client{
	Timeout: 10 * time.Second,
}

// Rule represents a single robots.txt rule.
type Rule struct {
	pattern string // URL pattern (supports * wildcards and $ anchor)
	allow   bool   // true = allow, false = disallow
}

// RobotsTxt represents parsed robots.txt rules.
type RobotsTxt struct {
	rules    []Rule   // Crawl rules
	Sitemaps []string // URLs of sitemap files
}

// ParseRobotsTxt parses robots.txt content into a RobotsTxt struct.
func ParseRobotsTxt(text string) *RobotsTxt {
	rules := make([]Rule, 0, 32)
	sitemaps := make([]string, 0, 4)
	inOurSection := false

	for _, line := range strings.Split(text, "\n") {
		l := strings.TrimSpace(line)
		// Skip empty lines and comments
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}

		lower := strings.ToLower(l)

		// Check User-Agent directive
		if agent := extractRobotsDirective(lower, "user-agent:"); agent != "" {
			// Check if this section applies to us
			inOurSection = agent == "*" || strings.Contains(agent, "quickcrawl")
			continue
		}

		// Check Sitemap directive
		if sitemap := extractRobotsDirective(lower, "sitemap:"); sitemap != "" {
			sitemaps = append(sitemaps, sitemap)
			continue
		}

		// Only process rules in our section
		if inOurSection {
			if path := extractRobotsDirective(lower, "disallow:"); path != "" {
				rules = append(rules, Rule{pattern: path, allow: false})
			} else if path := extractRobotsDirective(lower, "allow:"); path != "" {
				rules = append(rules, Rule{pattern: path, allow: true})
			}
		}
	}

	return &RobotsTxt{rules: rules, Sitemaps: sitemaps}
}

// extractRobotsDirective extracts the value from a robots.txt directive line.
// For example, "  disallow: /private  " returns "/private".
func extractRobotsDirective(line string, prefix string) string {
	if strings.HasPrefix(line, prefix) {
		value := line[len(prefix):]
		value = strings.TrimSpace(value)
		// Strip trailing comments
		if idx := strings.Index(value, "#"); idx != -1 {
			value = strings.TrimSpace(value[:idx])
		}
		return value
	}
	return ""
}

// IsAllowed checks if a path is allowed to be crawled.
// Returns true if the path is allowed, false if disallowed.
func (r *RobotsTxt) IsAllowed(path string) bool {
	var bestMatch *Rule
	bestLen := 0

	// Find the most specific matching rule
	for i := range r.rules {
		rule := &r.rules[i]
		if pathMatchesPattern(path, rule.pattern) {
			ruleLen := countSignificantPatternChars(rule.pattern)
			// Prefer longer (more specific) matches, or allow over disallow on ties
			if ruleLen > bestLen || (ruleLen == bestLen && rule.allow) {
				bestLen = ruleLen
				bestMatch = rule
			}
		}
	}

	// No matching rule means allowed
	if bestMatch == nil {
		return true
	}
	return bestMatch.allow
}

// FetchRobotsTxt fetches and parses robots.txt from the given origin.
func FetchRobotsTxt(origin, userAgent string, proxy *string) *RobotsTxt {
	robotsURL := strings.TrimRight(origin, "/") + "/robots.txt"
	u, err := url.Parse(robotsURL)
	if err != nil {
		return &RobotsTxt{}
	}

	client := robotsClient
	if proxy != nil && *proxy != "" {
		if proxyURL, parseErr := url.Parse(*proxy); parseErr == nil {
			transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
			client = &http.Client{Transport: transport, Timeout: 10 * time.Second}
		}
	}

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return &RobotsTxt{}
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	resp, err := client.Do(req)
	if err != nil {
		return &RobotsTxt{}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return &RobotsTxt{}
	}

	return ParseRobotsTxt(string(body))
}

// countSignificantPatternChars returns the effective length of a pattern
// (counting only significant characters, not * and $).
func countSignificantPatternChars(pattern string) int {
	count := 0
	for _, c := range pattern {
		if c != '*' && c != '$' {
			count++
		}
	}
	return count
}

// pathMatchesPattern checks if a path matches a robots.txt pattern.
// Supports * wildcard (any characters) and $ anchor (end of path).
func pathMatchesPattern(path string, pattern string) bool {
	// Check for $ anchor (end of path)
	anchoredEnd := strings.HasSuffix(pattern, "$")
	pattern = strings.TrimSuffix(pattern, "$")

	// No wildcards - simple prefix or exact match
	if !strings.Contains(pattern, "*") {
		if anchoredEnd {
			return path == pattern
		}
		return strings.HasPrefix(path, pattern)
	}

	// Wildcard pattern - match segments
	segments := strings.Split(pattern, "*")
	pos := 0

	for i, segment := range segments {
		if segment == "" {
			continue
		}
		if i == 0 {
			// First segment must match at start
			if !strings.HasPrefix(path[pos:], segment) {
				return false
			}
			pos += len(segment)
		} else {
			// Subsequent segments can appear anywhere
			idx := strings.Index(path[pos:], segment)
			if idx == -1 {
				return false
			}
			pos += idx + len(segment)
		}
	}

	// If anchored, must match to end
	if anchoredEnd {
		return pos == len(path)
	}
	return true
}
