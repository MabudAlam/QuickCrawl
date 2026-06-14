package crawler

import (
	"strings"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/utils"
)

func addRandomJitter(base time.Duration, factor float64) time.Duration {
	if factor <= 0.0 || base == 0 {
		return base
	}
	factor = utils.MinFloat64(factor, 1.0)
	rng := (float64(time.Now().UnixNano()%2000) - 1000) / 1000.0
	delta := base.Seconds() * factor * rng
	secs := utils.MaxFloat64(base.Seconds()+delta, 0)
	return time.Duration(secs * 1e9)
}

func isSafeURL(urlStr string) bool {
	_, err := utils.ValidateURL(urlStr)
	return err == nil
}

func normalizeURL(raw string) string {
	withoutFragment := raw
	if idx := strings.Index(raw, "#"); idx != -1 {
		withoutFragment = raw[:idx]
	}
	trimmed := strings.TrimSuffix(withoutFragment, "/")
	return strings.ToLower(trimmed)
}

func newBool(b bool) *bool {
	return &b
}

func maxIntValue(a, b int) int {
	return utils.MaxInt(a, b)
}

func newDomainRateLimiter(domain string, rps float64) *RateLimiter {
	return NewRateLimiter(rps)
}