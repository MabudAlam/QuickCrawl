package crawler

import (
	"strings"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/common"
)

// addRandomJitter adds random variation to a sleep duration.
// This helps avoid detection by introducing human-like variability.
// The factor controls how much randomness is added (0.0 to 1.0).
func addRandomJitter(base time.Duration, factor float64) time.Duration {
	if factor <= 0.0 || base == 0 {
		return base
	}
	factor = common.MinFloat64(factor, 1.0)
	// Random value between -1 and 1
	rng := (float64(time.Now().UnixNano()%2000) - 1000) / 1000.0
	delta := base.Seconds() * factor * rng
	secs := common.MaxFloat64(base.Seconds()+delta, 0)
	return time.Duration(secs * 1e9)
}

// isSafeURL checks if a URL is valid for crawling (safe and allowed).
func isSafeURL(urlStr string) bool {
	_, err := common.ValidateURL(urlStr)
	return err == nil
}

// normalizeURL normalizes a URL for consistent comparison.
// Removes fragment, trailing slash, and lowercases the URL.
func normalizeURL(raw string) string {
	withoutFragment := raw
	if idx := strings.Index(raw, "#"); idx != -1 {
		withoutFragment = raw[:idx]
	}
	trimmed := strings.TrimSuffix(withoutFragment, "/")
	return strings.ToLower(trimmed)
}

// newBool returns a pointer to a boolean value.
func newBool(b bool) *bool {
	return &b
}

// maxIntValue returns the larger of two integers.
func maxIntValue(a, b int) int {
	return common.MaxInt(a, b)
}

// newDomainRateLimiter creates a rate limiter for a domain.
func newDomainRateLimiter(domain string, rps float64) *RateLimiter {
	return NewRateLimiter(rps)
}
