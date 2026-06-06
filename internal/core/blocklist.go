// Package core provides the clean, chromedp-based reimplementation of the
// legacy renderer.
//
// File: blocklist.go
//
// The blocklist combines a hardcoded set of global patterns (analytics,
// trackers, ad networks) with optional caller-supplied custom patterns and
// optional resource-type filters. It is used by the browser rendering path
// to drop unwanted sub-requests at the Fetch domain level — see
// fetchWithCDPBrowser and the fetchFetchBlockAction helper below.
//
// Pattern syntax (delegated to pkg/pattern):
//
//   - Exact (no prefix): case-insensitive substring match
//   - Wildcard (*...*): case-insensitive wildcard match
//   - Regexp (~...): case-sensitive regular expression
//   - Regexp CI (~*...): case-insensitive regular expression
//
// All non-regexp patterns are lowercased once at construction time so the
// hot path (IsBlocked) only ever compares against a lowercased URL. The
// cost of IsBlocked is O(P) where P is the number of compiled patterns and
// the patterns are simple substring/wildcard/regexp matches — for the
// default global list (32 patterns) this is on the order of a few
// microseconds per URL, which is well under the cost of the CDP round-trip
// to send FailRequest.
package core

import (
	"strings"

	"github.com/MabudAlam/quickcrawl/pkg/pattern"
)

// globalBlockedPatterns is the hardcoded list of patterns to block across
// all browser-rendered requests. These are primarily analytics, tracking,
// and third-party services that do not contribute to the scraped content
// and significantly slow down page loads.
//
// Patterns use * as a wildcard for substring matching against full URLs.
// Match the engine's behaviour exactly so operators can copy-paste config
// between the two codebases.
var globalBlockedPatterns = []string{
	"*2mdn.net*",
	"*adobestats.com*",
	"*adsappier.com*",
	"*affirm.com*",
	"*ampproject.org*",
	"*braintree-api.com*",
	"*braintreegateway.com*",
	"*chatra.io*",
	"*convertexperiments.com*",
	"*doubleclick.net*",
	"*estorecontent.com*",
	"*google-analytics.com*",
	"*googleadservices.com*",
	"*googleapis.com*",
	"*googlesyndication.com*",
	"*googletagservices.com*",
	"*googletagmanager.com*",
	"*googlevideo.com*",
	"*gstatic.com*",
	"*facebook.com*",
	"*lexx.me*",
	"*paypal.com*",
	"*paypalobjects.com*",
	"*pointandplace.com*",
	"*typekit.net*",
	"*twitter.com*",
	"*hotjar.com*",
	"*clarity.ms*",
	"*analytics.google.com*",
	"*youtube.com*",
	"*listrakbi.com*",
	"*static.cloudflareinsights.com*",
}

// Blocklist holds compiled blocking rules for a single browser render.
//
// A Blocklist is safe to read concurrently from multiple goroutines once
// constructed (the underlying *pattern.Pattern is immutable). It is
// intended to be built once per request by the renderer and read once per
// Fetch.requestPaused event from the chromedp event listener goroutine.
type Blocklist struct {
	compiledPatterns     []*pattern.Pattern
	originalPatterns     []string
	blockedResourceTypes map[string]struct{}
}

// NewBlocklist creates a new blocklist combining global rules and custom
// patterns. Resource-type filtering is disabled.
//
// This is the constructor most callers want.
func NewBlocklist(customPatterns []string) *Blocklist {
	return NewBlocklistWithResourceTypes(customPatterns, nil)
}

// NewBlocklistWithResourceTypes creates a new blocklist with URL patterns
// and resource type filtering.
//
// Custom patterns are appended after the global list so they are checked
// last (the first matching pattern short-circuits IsBlocked, so order
// matters only for the rare case of overlapping patterns). Empty /
// whitespace-only patterns are dropped at construction time. Patterns that
// fail to compile (malformed regex, etc.) are dropped silently to keep the
// scraper resilient to misconfiguration.
func NewBlocklistWithResourceTypes(customPatterns []string, resourceTypes []string) *Blocklist {
	allPatterns := make([]string, 0, len(globalBlockedPatterns)+len(customPatterns))
	allPatterns = append(allPatterns, globalBlockedPatterns...)
	allPatterns = append(allPatterns, customPatterns...)

	bl := &Blocklist{
		compiledPatterns:     make([]*pattern.Pattern, 0, len(allPatterns)),
		originalPatterns:     allPatterns,
		blockedResourceTypes: make(map[string]struct{}),
	}

	for _, raw := range allPatterns {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		// Lowercase non-regexp patterns once at compile time so the
		// hot path can match against a pre-lowercased URL. Regexp
		// patterns preserve case (the regex itself controls
		// case-sensitivity via the ~* prefix or an inline (?i)).
		canonical := raw
		if !strings.HasPrefix(canonical, "~") {
			canonical = strings.ToLower(canonical)
		}
		compiled, err := pattern.Compile(canonical)
		if err != nil {
			continue
		}
		bl.compiledPatterns = append(bl.compiledPatterns, compiled)
	}

	for _, rt := range resourceTypes {
		rt = strings.TrimSpace(rt)
		if rt == "" {
			continue
		}
		bl.blockedResourceTypes[rt] = struct{}{}
	}

	return bl
}

// IsBlocked reports whether the given request URL should be blocked.
//
// URL is lowercased once per call. The loop short-circuits on the first
// matching pattern. This function is called once per browser sub-request
// (images, scripts, fonts, XHRs, etc.) so the cost adds up; the per-call
// budget is dominated by the lowercasing step, not the pattern loop.
func (bl *Blocklist) IsBlocked(requestURL string) bool {
	if bl == nil || len(bl.compiledPatterns) == 0 {
		return false
	}

	lowercased := strings.ToLower(requestURL)

	for _, compiled := range bl.compiledPatterns {
		url := lowercased
		if compiled.Type == pattern.PatternTypeRegexp {
			// Regexp patterns preserve case (case-sensitivity is
			// controlled by the ~* prefix or an inline (?i) in the
			// pattern itself). Match against the original URL.
			url = requestURL
		}
		if compiled.Match(url) {
			return true
		}
	}
	return false
}

// IsResourceTypeBlocked reports whether the given CDP resource type (e.g.
// "Image", "Media", "Font", "Script", "XHR") should be blocked.
//
// Returns false if no resource types were configured, so the default
// Blocklist (NewBlocklist without resource types) is a pure URL filter.
func (bl *Blocklist) IsResourceTypeBlocked(resourceType string) bool {
	if bl == nil || len(bl.blockedResourceTypes) == 0 {
		return false
	}
	_, blocked := bl.blockedResourceTypes[resourceType]
	return blocked
}

// PatternCount returns the number of compiled URL patterns. Useful for
// logging and metrics.
func (bl *Blocklist) PatternCount() int {
	if bl == nil {
		return 0
	}
	return len(bl.compiledPatterns)
}
