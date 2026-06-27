package brand

import (
	"strings"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/PuerkitoBio/goquery"
)

var socialPatterns = map[string][]string{
	"linkedin": {
		"linkedin.com/company/",
		"linkedin.com/in/",
	},
	"x": {
		"x.com/",
		"twitter.com/",
	},
	"github": {
		"github.com/",
	},
	"discord": {
		"discord.com/invite/",
		"discord.com/channels/",
		"discord.gg/",
	},
	"facebook": {
		"facebook.com/pages/",
		"facebook.com/",
	},
	"instagram": {
		"instagram.com/",
	},
	"youtube": {
		"youtube.com/channel/",
		"youtube.com/c/",
		"youtube.com/@",
	},
	"tiktok": {
		"tiktok.com/@",
		"tiktok.com/channel/",
	},
}

var excludePatterns = []string{
	"status=",
	"share=",
	"intent/tweet",
	"followers",
	"following",
	"profile",
	"photos",
	"posts",
	"tagged",
	"explore",
	"notifications",
	"settings",
	"messages",
	"search",
	"/status/",
	"facebook.com/photo",
	"facebook.com/event",
	"facebook.com/groups",
	"facebook.com/plugins",
	"instagram.com/explore",
	"instagram.com/direct",
	"youtube.com/watch",
	"youtube.com/playlist",
	"youtube.com/shorts",
	"tiktok.com/discover",
	"github.com/issues",
	"github.com/pulls",
	"github.com/notifications",
	"linkedin.com/feed",
	"linkedin.com/messaging",
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
		if href == "" {
			return
		}

		hrefLower := strings.ToLower(href)

		for _, exclude := range excludePatterns {
			if strings.Contains(hrefLower, exclude) {
				return
			}
		}

		for socialType, patterns := range socialPatterns {
			for _, pattern := range patterns {
				if strings.Contains(hrefLower, pattern) {
					normalizedURL := normalizeSocialURL(href, socialType)
					if normalizedURL != "" && !seen[normalizedURL] {
						seen[normalizedURL] = true
						socials = append(socials, types.SocialLink{
							Type: socialType,
							URL:  normalizedURL,
						})
					}
					break
				}
			}
		}
	})

	return socials
}

func normalizeSocialURL(href, socialType string) string {
	switch socialType {
	case "linkedin":
		if strings.Contains(href, "linkedin.com/company/") || strings.Contains(href, "linkedin.com/in/") {
			if !strings.HasPrefix(href, "http") {
				return "https://" + href
			}
			return href
		}
	case "x":
		if strings.Contains(href, "x.com/") || strings.Contains(href, "twitter.com/") {
			if !strings.HasPrefix(href, "http") {
				return "https://" + href
			}
			return href
		}
	case "github":
		if strings.Contains(href, "github.com/") && !strings.Contains(href, "github.com/settings") {
			githubURL := href
			if !strings.HasPrefix(href, "http") {
				githubURL = "https://" + href
			}
			parts := strings.Split(strings.TrimPrefix(githubURL, "https://github.com/"), "/")
			if len(parts) >= 1 && parts[0] != "" {
				return "https://github.com/" + parts[0]
			}
			return githubURL
		}
	case "discord":
		if strings.Contains(href, "discord.com/invite/") || strings.Contains(href, "discord.com/channels/") || strings.Contains(href, "discord.gg/") {
			if !strings.HasPrefix(href, "http") {
				return "https://" + href
			}
			return href
		}
	case "facebook":
		if strings.Contains(href, "facebook.com/pages/") || strings.Contains(href, "facebook.com/") {
			if !strings.HasPrefix(href, "http") {
				return "https://" + href
			}
			return href
		}
	case "instagram":
		if strings.Contains(href, "instagram.com/") && !strings.Contains(href, "instagram.com/explore") {
			if !strings.HasPrefix(href, "http") {
				return "https://" + href
			}
			return href
		}
	case "youtube":
		if strings.Contains(href, "youtube.com/channel/") || strings.Contains(href, "youtube.com/c/") || strings.Contains(href, "youtube.com/@") {
			if !strings.HasPrefix(href, "http") {
				return "https://" + href
			}
			return href
		}
	case "tiktok":
		if strings.Contains(href, "tiktok.com/@") || strings.Contains(href, "tiktok.com/channel/") {
			if !strings.HasPrefix(href, "http") {
				return "https://" + href
			}
			return href
		}
	}
	return ""
}
