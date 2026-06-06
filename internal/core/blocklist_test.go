package core

import (
	"strings"
	"testing"
)

func TestNewBlocklist_GlobalPatterns(t *testing.T) {
	bl := NewBlocklist(nil)
	if bl.PatternCount() != len(globalBlockedPatterns) {
		t.Fatalf("expected %d global patterns, got %d", len(globalBlockedPatterns), bl.PatternCount())
	}
}

func TestNewBlocklist_EmptyCustom(t *testing.T) {
	bl := NewBlocklist([]string{})
	if bl.PatternCount() != len(globalBlockedPatterns) {
		t.Fatalf("expected %d global patterns, got %d", len(globalBlockedPatterns), bl.PatternCount())
	}
}

func TestNewBlocklist_CustomWildcard(t *testing.T) {
	bl := NewBlocklist([]string{"*tracker.example.com*", "*ads.example.org/*"})
	if bl.PatternCount() != len(globalBlockedPatterns)+2 {
		t.Fatalf("expected %d patterns, got %d", len(globalBlockedPatterns)+2, bl.PatternCount())
	}
	if !bl.IsBlocked("https://tracker.example.com/pixel.gif") {
		t.Error("expected tracker.example.com URL to be blocked")
	}
	if !bl.IsBlocked("https://cdn.ads.example.org/banner.png") {
		t.Error("expected ads.example.org URL to be blocked")
	}
}

func TestNewBlocklist_GlobalDomains(t *testing.T) {
	bl := NewBlocklist(nil)
	cases := []string{
		"https://www.google-analytics.com/collect?v=1",
		"https://www.googletagmanager.com/gtm.js",
		"https://www.facebook.com/tr?id=123",
		"https://static.hotjar.com/c/hotjar-1234.js",
		"https://www.youtube.com/embed/abc123",
	}
	for _, url := range cases {
		if !bl.IsBlocked(url) {
			t.Errorf("expected %q to be blocked by global list", url)
		}
	}
}

func TestNewBlocklist_NonBlocked(t *testing.T) {
	bl := NewBlocklist(nil)
	cases := []string{
		"https://example.com/page",
		"https://docs.tinyfish.ai/article/abc",
		"https://api.openai.com/v1/chat",
	}
	for _, url := range cases {
		if bl.IsBlocked(url) {
			t.Errorf("did not expect %q to be blocked", url)
		}
	}
}

func TestNewBlocklist_CaseInsensitive(t *testing.T) {
	bl := NewBlocklist([]string{"*Example.COM*"})
	if !bl.IsBlocked("https://EXAMPLE.com/path") {
		t.Error("expected case-insensitive match to block")
	}
	if !bl.IsBlocked("https://example.com/Path") {
		t.Error("expected mixed case URL to be blocked")
	}
}

func TestNewBlocklist_RegexpCaseSensitive(t *testing.T) {
	bl := NewBlocklist([]string{"~^https://[A-Z]+.example.com/.*$"})
	if !bl.IsBlocked("https://ABC.example.com/foo") {
		t.Error("expected uppercase host to match case-sensitive regex")
	}
	if bl.IsBlocked("https://abc.example.com/foo") {
		t.Error("did not expect lowercase host to match case-sensitive regex")
	}
}

func TestNewBlocklist_RegexpCaseInsensitive(t *testing.T) {
	bl := NewBlocklist([]string{"~*tracking|analytics*"})
	if !bl.IsBlocked("https://example.com/tracking/pixel.js") {
		t.Error("expected mixed case substring to match")
	}
	if !bl.IsBlocked("https://example.com/Analytics/script.js") {
		t.Error("expected uppercase substring to match")
	}
}

func TestNewBlocklist_EmptyAndWhitespace(t *testing.T) {
	bl := NewBlocklist([]string{"", "  ", "\t", "*real-tracker.com*"})
	// All empty/whitespace patterns should be dropped silently.
	if bl.PatternCount() != len(globalBlockedPatterns)+1 {
		t.Fatalf("expected %d patterns after dropping empties, got %d", len(globalBlockedPatterns)+1, bl.PatternCount())
	}
	if !bl.IsBlocked("https://real-tracker.com/pixel") {
		t.Error("expected real-tracker.com URL to be blocked")
	}
}

func TestNewBlocklist_InvalidRegexp(t *testing.T) {
	bl := NewBlocklist([]string{"~[invalid(regex", "*valid-tracker.com*"})
	// Invalid regex should be dropped silently; the valid pattern should still compile.
	if !bl.IsBlocked("https://valid-tracker.com/pixel") {
		t.Error("expected valid-tracker.com URL to be blocked")
	}
}

func TestNewBlocklist_ResourceTypeBlocking(t *testing.T) {
	bl := NewBlocklistWithResourceTypes(nil, []string{"Image", "Media", "Font"})
	if !bl.IsResourceTypeBlocked("Image") {
		t.Error("expected Image resource type to be blocked")
	}
	if !bl.IsResourceTypeBlocked("Font") {
		t.Error("expected Font resource type to be blocked")
	}
	if bl.IsResourceTypeBlocked("Document") {
		t.Error("did not expect Document resource type to be blocked")
	}
}

func TestNewBlocklist_ResourceTypeTrimsWhitespace(t *testing.T) {
	bl := NewBlocklistWithResourceTypes(nil, []string{" Image ", "  Media\t"})
	if !bl.IsResourceTypeBlocked("Image") {
		t.Error("expected trimmed Image resource type to be blocked")
	}
	if !bl.IsResourceTypeBlocked("Media") {
		t.Error("expected trimmed Media resource type to be blocked")
	}
}

func TestBlocklist_ResourceTypeEmptyDefault(t *testing.T) {
	bl := NewBlocklist(nil)
	if bl.IsResourceTypeBlocked("Image") {
		t.Error("default blocklist should not block any resource types")
	}
}

func TestBlocklist_NilSafe(t *testing.T) {
	var bl *Blocklist
	if bl.IsBlocked("https://example.com") {
		t.Error("nil blocklist should not block")
	}
	if bl.IsResourceTypeBlocked("Image") {
		t.Error("nil blocklist should not block resource types")
	}
	if bl.PatternCount() != 0 {
		t.Error("nil blocklist should report 0 patterns")
	}
}

func TestBlocklist_RealWorldScenarios(t *testing.T) {
	bl := NewBlocklist([]string{"*my-custom-tracker.io*"})

	// Block: global + custom.
	if !bl.IsBlocked("https://www.google-analytics.com/collect") {
		t.Error("expected GA to be blocked")
	}
	if !bl.IsBlocked("https://my-custom-tracker.io/event") {
		t.Error("expected custom tracker to be blocked")
	}

	// Pass: not in any list.
	passes := []string{
		"https://example.com/article",
		"https://cdn.jsdelivr.net/npm/jquery.js",
		"https://unpkg.com/react@18/umd/react.production.min.js",
	}
	for _, url := range passes {
		if bl.IsBlocked(url) {
			t.Errorf("did not expect %q to be blocked", url)
		}
	}
}

func TestBlocklist_OriginalPatternsPreserved(t *testing.T) {
	bl := NewBlocklist([]string{"*foo.com*", "*bar.com*"})
	foundFoo := false
	foundBar := false
	for _, p := range bl.originalPatterns {
		if strings.Contains(p, "foo.com") {
			foundFoo = true
		}
		if strings.Contains(p, "bar.com") {
			foundBar = true
		}
	}
	if !foundFoo || !foundBar {
		t.Error("expected originalPatterns to retain user-supplied strings")
	}
}
