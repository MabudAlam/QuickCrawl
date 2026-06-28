package brand

import (
	"net/url"
	"strings"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/PuerkitoBio/goquery"
)

var excludePatterns = []string{
	"status=",
	"share=",
	"intent/tweet",
	"/status/",
	"facebook.com/photo",
	"facebook.com/event",
	"facebook.com/groups",
	"facebook.com/plugins",
	"facebook.com/sharer",
	"instagram.com/explore",
	"instagram.com/direct",
	"youtube.com/watch",
	"youtube.com/playlist",
	"youtube.com/shorts",
	"tiktok.com/discover",
	"tiktok.com/tag",
	"github.com/issues",
	"github.com/pulls",
	"github.com/notifications",
	"github.com/settings",
	"github.com/sponsors",
	"linkedin.com/feed",
	"linkedin.com/messaging",
	"linkedin.com/notifications",
	"linkedin.com/posts/",
	"linkedin.com/pulse/",
}

type platform struct {
	socialType string
	hosts      []string
	matcher    func(path string) (string, bool)
}

var platforms = []platform{
	{"linkedin", []string{"linkedin.com"}, matchLinkedIn},
	{"x", []string{"x.com", "twitter.com"}, matchX},
	{"github", []string{"github.com"}, matchGitHub},
	{"discord", []string{"discord.com"}, matchDiscordCom},
	{"discord", []string{"discord.gg"}, matchFlat(nil)},
	{"facebook", []string{"facebook.com"}, matchFacebook},
	{"instagram", []string{"instagram.com"}, matchInstagram},
	{"youtube", []string{"youtube.com"}, matchYouTube},
	{"tiktok", []string{"tiktok.com"}, matchTikTok},
}

func extractSocials(doc *goquery.Document) []types.SocialLink {
	var socials []types.SocialLink
	seen := make(map[string]bool)

	socialSelectors := []string{
		"footer a",
		"header a",
		".footer a",
		".header a",
		"[class*='footer'] a",
		"[class*='social'] a",
		"[class*='contact'] a",
		".social-links a",
		".social-media a",
	}

	doc.Find(strings.Join(socialSelectors, ", ")).Each(func(i int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		href = strings.TrimSpace(href)
		if href == "" {
			return
		}

		hrefLower := strings.ToLower(href)
		for _, exclude := range excludePatterns {
			if strings.Contains(hrefLower, exclude) {
				return
			}
		}

		socialType, normalizedURL := matchSocialURL(href)
		if socialType == "" || normalizedURL == "" {
			return
		}
		if !seen[normalizedURL] {
			seen[normalizedURL] = true
			socials = append(socials, types.SocialLink{
				Type: socialType,
				URL:  normalizedURL,
			})
		}
	})

	return socials
}

func matchSocialURL(href string) (socialType, normalizedURL string) {
	parsed, ok := parseHrefURL(href)
	if !ok {
		return "", ""
	}
	host := strings.ToLower(strings.TrimPrefix(parsed.Host, "www."))
	path := strings.TrimSuffix(parsed.Path, "/")
	if path == "" {
		path = "/"
	}

	for _, p := range platforms {
		hostOK := false
		for _, h := range p.hosts {
			if hostMatches(host, h) {
				hostOK = true
				break
			}
		}
		if !hostOK {
			continue
		}
		if account, ok := p.matcher(path); ok && account != "" {
			return p.socialType, "https://" + p.hosts[0] + "/" + account
		}
	}
	return "", ""
}

func parseHrefURL(href string) (*url.URL, bool) {
	candidate := href
	switch {
	case strings.HasPrefix(candidate, "//"):
		candidate = "https:" + candidate
	case !strings.Contains(candidate, "://"):
		candidate = "https://" + candidate
	}
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Host == "" {
		return nil, false
	}
	return parsed, true
}

func hostMatches(host, domain string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}

func firstSegment(s string) string {
	if idx := strings.IndexByte(s, '/'); idx != -1 {
		return s[:idx]
	}
	return s
}

func stripPrefixCI(path, prefix string) (string, bool) {
	if len(path) < len(prefix) {
		return "", false
	}
	if strings.EqualFold(path[:len(prefix)], prefix) {
		return path[len(prefix):], true
	}
	return "", false
}

func matchFlat(excluded map[string]bool) func(string) (string, bool) {
	return func(path string) (string, bool) {
		trimmed := strings.TrimPrefix(path, "/")
		if trimmed == "" {
			return "", false
		}
		account := firstSegment(trimmed)
		if account == "" || excluded[strings.ToLower(account)] {
			return "", false
		}
		return account, true
	}
}

func matchLinkedIn(path string) (string, bool) {
	for _, prefix := range []string{"/company/", "/in/", "/school/", "/showcase/"} {
		if rest, ok := stripPrefixCI(path, prefix); ok && rest != "" {
			return strings.TrimPrefix(prefix, "/") + firstSegment(rest), true
		}
	}
	return "", false
}

func matchX(path string) (string, bool) {
	trimmed := strings.TrimPrefix(path, "/")
	if trimmed == "" {
		return "", false
	}
	account := firstSegment(trimmed)
	switch strings.ToLower(account) {
	case "i", "home", "explore", "notifications", "messages", "settings", "search", "compose":
		return "", false
	}
	return account, true
}

func matchGitHub(path string) (string, bool) {
	trimmed := strings.TrimPrefix(path, "/")
	if trimmed == "" {
		return "", false
	}
	account := firstSegment(trimmed)
	switch strings.ToLower(account) {
	case "settings", "issues", "pulls", "notifications", "sponsors", "marketplace":
		return "", false
	}
	return account, true
}

func matchDiscordCom(path string) (string, bool) {
	if rest, ok := stripPrefixCI(path, "/invite/"); ok && rest != "" {
		return "invite/" + firstSegment(rest), true
	}
	if rest, ok := stripPrefixCI(path, "/channels/"); ok && rest != "" {
		segs := strings.SplitN(rest, "/", 3)
		if len(segs) >= 2 && segs[1] != "" {
			return "channels/" + segs[0] + "/" + segs[1], true
		}
		return "channels/" + segs[0], true
	}
	return "", false
}

func matchFacebook(path string) (string, bool) {
	if rest, ok := stripPrefixCI(path, "/pages/"); ok && rest != "" {
		segs := strings.SplitN(rest, "/", 3)
		if len(segs) >= 2 && segs[1] != "" {
			return "pages/" + segs[0] + "/" + segs[1], true
		}
		return "pages/" + segs[0], true
	}
	trimmed := strings.TrimPrefix(path, "/")
	if trimmed == "" {
		return "", false
	}
	account := firstSegment(trimmed)
	switch strings.ToLower(account) {
	case "groups", "marketplace", "watch", "gaming", "events":
		return "", false
	}
	return account, true
}

func matchInstagram(path string) (string, bool) {
	trimmed := strings.TrimPrefix(path, "/")
	if trimmed == "" {
		return "", false
	}
	account := firstSegment(trimmed)
	switch strings.ToLower(account) {
	case "explore", "direct", "accounts":
		return "", false
	}
	return account, true
}

func matchYouTube(path string) (string, bool) {
	for _, prefix := range []string{"/channel/", "/c/", "/user/"} {
		if rest, ok := stripPrefixCI(path, prefix); ok && rest != "" {
			return strings.TrimPrefix(prefix, "/") + firstSegment(rest), true
		}
	}
	trimmed := strings.TrimPrefix(path, "/")
	if strings.HasPrefix(trimmed, "@") {
		account := firstSegment(trimmed)
		if account != "@" {
			return account, true
		}
	}
	return "", false
}

func matchTikTok(path string) (string, bool) {
	if rest, ok := stripPrefixCI(path, "/channel/"); ok && rest != "" {
		return "channel/" + firstSegment(rest), true
	}
	trimmed := strings.TrimPrefix(path, "/")
	if strings.HasPrefix(trimmed, "@") {
		account := firstSegment(trimmed)
		if account != "@" {
			return account, true
		}
	}
	return "", false
}