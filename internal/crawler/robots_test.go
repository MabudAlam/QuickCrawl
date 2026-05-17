package crawler

import (
	"testing"
)

func TestParseRobotsTxtAllowAll(t *testing.T) {
	robotsTxt := `
User-agent: *
Allow: /

Sitemap: https://example.com/sitemap.xml
`
	robots := ParseRobotsTxt(robotsTxt, "Mozilla/5.0")

	if !robots.IsAllowed("/") {
		t.Error("expected root path to be allowed")
	}
	if !robots.IsAllowed("/page") {
		t.Error("expected /page to be allowed")
	}
	if !robots.IsAllowed("/deep/nested/path") {
		t.Error("expected deep nested path to be allowed")
	}
}

func TestParseRobotsTxtDisallowAll(t *testing.T) {
	robotsTxt := `
User-agent: *
Disallow: /

Sitemap: https://example.com/sitemap.xml
`
	robots := ParseRobotsTxt(robotsTxt, "Mozilla/5.0")

	if robots.IsAllowed("/") {
		t.Error("expected root path to be disallowed")
	}
	if robots.IsAllowed("/page") {
		t.Error("expected /page to be disallowed")
	}
}

func TestParseRobotsTxtDisallowSpecificPaths(t *testing.T) {
	robotsTxt := `
User-agent: *
Disallow: /admin/
Disallow: /private/
Disallow: /api/

Allow: /

Sitemap: https://example.com/sitemap.xml
`
	robots := ParseRobotsTxt(robotsTxt, "Mozilla/5.0")

	if !robots.IsAllowed("/") {
		t.Error("expected / to be allowed")
	}
	if !robots.IsAllowed("/public/page") {
		t.Error("expected /public/page to be allowed")
	}
	if robots.IsAllowed("/admin/") {
		t.Error("expected /admin/ to be disallowed")
	}
	if robots.IsAllowed("/admin/dashboard") {
		t.Error("expected /admin/dashboard to be disallowed")
	}
	if robots.IsAllowed("/private/") {
		t.Error("expected /private/ to be disallowed")
	}
	if robots.IsAllowed("/api/v1/users") {
		t.Error("expected /api/v1/users to be disallowed")
	}
}

func TestParseRobotsTxtSpecificBotAllowed(t *testing.T) {
	robotsTxt := `
User-agent: *
Disallow: /

User-agent: QuickCrawl
Allow: /

User-agent: FeatsAIBot
Allow: /api/public/
Disallow: /

Sitemap: https://example.com/sitemap.xml
`
	robots := ParseRobotsTxt(robotsTxt, "QuickCrawl")

	if !robots.IsAllowed("/") {
		t.Error("QuickCrawl should be allowed on root")
	}
	if !robots.IsAllowed("/admin/secret") {
		t.Error("QuickCrawl should be allowed on /admin/secret since it has specific allow")
	}
}

func TestParseRobotsTxtSpecificBotDisallowed(t *testing.T) {
	robotsTxt := `
User-agent: *
Allow: /

User-agent: BadBot
Disallow: /

Sitemap: https://example.com/sitemap.xml
`
	robots := ParseRobotsTxt(robotsTxt, "BadBot")

	if robots.IsAllowed("/") {
		t.Error("BadBot should be disallowed on root")
	}
	if robots.IsAllowed("/public") {
		t.Error("BadBot should be disallowed on /public")
	}
}

func TestParseRobotsTxtWildcardMatching(t *testing.T) {
	robotsTxt := `
User-agent: *
Disallow: /admin/*
Disallow: /api/*/users

Allow: /
`
	robots := ParseRobotsTxt(robotsTxt, "Mozilla/5.0")

	if !robots.IsAllowed("/") {
		t.Error("expected / to be allowed")
	}
	if robots.IsAllowed("/admin/panel") {
		t.Error("expected /admin/panel to be disallowed (wildcard)")
	}
	if robots.IsAllowed("/admin/settings") {
		t.Error("expected /admin/settings to be disallowed (wildcard)")
	}
	if robots.IsAllowed("/api/v1/users") {
		t.Error("expected /api/v1/users to be disallowed (wildcard)")
	}
	if !robots.IsAllowed("/api/v1/products") {
		t.Error("expected /api/v1/products to be allowed")
	}
}

func TestParseRobotsTxtCrawlDelay(t *testing.T) {
	robotsTxt := `
User-agent: *
Crawl-delay: 10

User-agent: FastBot
Crawl-delay: 1
`
	robots := ParseRobotsTxt(robotsTxt, "Mozilla/5.0")

	if !robots.IsAllowed("/") {
		t.Error("expected / to be allowed")
	}
}

func TestParseRobotsTxtEmptyUserAgent(t *testing.T) {
	robotsTxt := `
User-agent: *
Disallow: /private/
`
	robots := ParseRobotsTxt(robotsTxt, "")

	if !robots.IsAllowed("/") {
		t.Error("expected / to be allowed when no matching rule for empty UA")
	}
}

func TestParseRobotsTxtFetchWithEmptyUserAgent(t *testing.T) {
	robotsTxt := `
User-agent: *
Disallow: /

User-agent: quickcrawl
Allow: /
`
	robots := ParseRobotsTxt(robotsTxt, "")

	if robots.IsAllowed("/") {
		t.Error("empty UA should be disallowed since * disallows and no specific UA matches")
	}
}

func TestParseRobotsTxtFetchWithChromeUserAgent(t *testing.T) {
	robotsTxt := `
User-agent: *
Disallow: /

User-agent: quickcrawl
Allow: /
`
	chromeUA := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	robots := ParseRobotsTxt(robotsTxt, chromeUA)

	if robots.IsAllowed("/") {
		t.Error("Chrome UA should be disallowed by * rule (disallow: /)")
	}
}

func TestParseRobotsTxtFetchWithQuickCrawlUserAgent(t *testing.T) {
	robotsTxt := `
User-agent: *
Disallow: /

User-agent: quickcrawl
Allow: /
`
	robots := ParseRobotsTxt(robotsTxt, "quickcrawl")

	if !robots.IsAllowed("/") {
		t.Error("quickcrawl UA should be allowed")
	}
}

func TestParseRobotsTxtNoMatchingRule(t *testing.T) {
	robotsTxt := `
User-agent: KnownBot
Disallow: /secret/

User-agent: OtherBot
Allow: /
`
	robots := ParseRobotsTxt(robotsTxt, "UnknownBot")

	if !robots.IsAllowed("/") {
		t.Error("expected / to be allowed when no matching rule (falls back to * allow)")
	}
}

func TestParseRobotsTxtMultipleSitemaps(t *testing.T) {
	robotsTxt := `
User-agent: *
Allow: /

Sitemap: https://example.com/sitemap.xml
Sitemap: https://blog.example.com/sitemap.xml
Sitemap: https://shop.example.com/sitemap.xml
`
	robots := ParseRobotsTxt(robotsTxt, "Mozilla/5.0")

	if len(robots.Sitemaps) != 3 {
		t.Errorf("expected 3 sitemaps, got %d", len(robots.Sitemaps))
	}
}

func TestParseRobotsTxtCaseInsensitiveUserAgent(t *testing.T) {
	robotsTxt := `
User-agent: QUICKCRAWL
Disallow: /admin/

User-agent: *
Allow: /
`
	robots := ParseRobotsTxt(robotsTxt, "quickcrawl")

	if robots.IsAllowed("/admin/") {
		t.Error("expected case-insensitive UA matching to disallow /admin/")
	}
}

func TestParseRobotsTxtInvalidRobotsTxt(t *testing.T) {
	invalidTxt := "this is not valid robots.txt content @@!#%"
	robots := ParseRobotsTxt(invalidTxt, "Mozilla/5.0")

	if robots == nil {
		t.Error("ParseRobotsTxt should return valid RobotsTxt even for invalid content")
	}
	if !robots.IsAllowed("/") {
		t.Error("invalid robots.txt should default to allowing all")
	}
}

func TestParseRobotsTxtEmptyRobotsTxt(t *testing.T) {
	robots := ParseRobotsTxt("", "Mozilla/5.0")

	if robots == nil {
		t.Error("ParseRobotsTxt should return valid RobotsTxt even for empty content")
	}
	if !robots.IsAllowed("/") {
		t.Error("empty robots.txt should default to allowing all")
	}
}

func TestParseRobotsTxtNilData(t *testing.T) {
	robots := &RobotsTxt{userAgent: "test"}

	if !robots.IsAllowed("/") {
		t.Error("nil robots data should default to allowing all")
	}
}

func TestParseRobotsTxtMabudDev(t *testing.T) {
	robotsTxt := `
# robots.txt for https://www.mabud.dev

User-agent: *
Disallow: /

User-agent: FeatsAIBot
Allow: /

User-agent: quickcrawl
Allow: /

Sitemap: https://www.mabud.dev/sitemap.xml
`
	quickcrawlBot := ParseRobotsTxt(robotsTxt, "quickcrawl")
	chromeBot := ParseRobotsTxt(robotsTxt, "Mozilla/5.0")
	featsBot := ParseRobotsTxt(robotsTxt, "FeatsAIBot")
	unknownBot := ParseRobotsTxt(robotsTxt, "UnknownBot")

	if !quickcrawlBot.IsAllowed("/") {
		t.Error("quickcrawl should be allowed on mabud.dev")
	}
	if !featsBot.IsAllowed("/") {
		t.Error("FeatsAIBot should be allowed on mabud.dev")
	}
	if chromeBot.IsAllowed("/") {
		t.Error("Chrome (generic *) should be disallowed on mabud.dev")
	}
	if unknownBot.IsAllowed("/") {
		t.Error("UnknownBot should be disallowed (falls back to *)")
	}
}

func TestParseRobotsTxtSubstack(t *testing.T) {
	robotsTxt := `
User-agent: *
Disallow:

Sitemap: https://substack.com/sitemap.xml
`
	robots := ParseRobotsTxt(robotsTxt, "Mozilla/5.0")

	if !robots.IsAllowed("/") {
		t.Error("substack with empty Disallow should allow all")
	}
}

func TestParseRobotsTxtNotion(t *testing.T) {
	robotsTxt := `
User-agent: *
Disallow: /

User-agent: Twitterbot
Allow: /

User-agent: Facebookbot
Allow: /

Sitemap: https://www.notion.com/sitemap.xml
`
	robots := ParseRobotsTxt(robotsTxt, "Mozilla/5.0")

	if robots.IsAllowed("/") {
		t.Error("generic UA should be disallowed on notion")
	}
}

func TestParseRobotsTxtFeatsclub(t *testing.T) {
	robotsTxt := `
User-agent: *
Disallow: /api/
Disallow: /admin

Allow: /

Sitemap: https://www.featsclub.com/sitemap.xml
`
	robots := ParseRobotsTxt(robotsTxt, "Mozilla/5.0")

	if !robots.IsAllowed("/") {
		t.Error("/ should be allowed on featsclub")
	}
	if !robots.IsAllowed("/page/about") {
		t.Error("/page/about should be allowed on featsclub")
	}
	if robots.IsAllowed("/api/") {
		t.Error("/api/ should be disallowed on featsclub")
	}
	if robots.IsAllowed("/admin") {
		t.Error("/admin should be disallowed on featsclub")
	}
}

func TestParseRobotsTxtPartialPathDisallow(t *testing.T) {
	robotsTxt := `
User-agent: *
Disallow: /blog

Allow: /blog/public
Allow: /blog/about
`
	robots := ParseRobotsTxt(robotsTxt, "Mozilla/5.0")

	if robots.IsAllowed("/blog") {
		t.Error("/blog should be disallowed")
	}
	if !robots.IsAllowed("/blog/public") {
		t.Error("/blog/public should be allowed")
	}
	if !robots.IsAllowed("/blog/about") {
		t.Error("/blog/about should be allowed")
	}
	if robots.IsAllowed("/blog/private") {
		t.Error("/blog/private should be disallowed")
	}
}