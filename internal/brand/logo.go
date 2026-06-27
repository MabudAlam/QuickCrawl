package brand

import (
	"net/url"
	"path"
	"strings"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/PuerkitoBio/goquery"
)

func extractLogos(doc *goquery.Document, pageURL string) []types.BrandLogo {
	var logos []types.BrandLogo
	logoSet := make(map[string]bool)

	parsedURL, _ := url.Parse(pageURL)
	domain := ""
	if parsedURL != nil {
		domain = parsedURL.Hostname()
	}

	doc.Find("link").Each(func(i int, s *goquery.Selection) {
		rel, _ := s.Attr("rel")
		href, _ := s.Attr("href")
		typ, _ := s.Attr("type")
		sizesAttr, _ := s.Attr("sizes")

		if href == "" || rel == "" {
			return
		}

		relLower := strings.ToLower(rel)
		hasIcon := false
		isApple := false

		rtoks := strings.Fields(relLower)
		for _, t := range rtoks {
			switch t {
			case "icon":
				hasIcon = true
			case "apple-touch-icon", "apple-touch-icon-precomposed":
				isApple = true
			case "mask-icon":
				return
			}
		}

		if !hasIcon && !isApple {
			return
		}

		base := parsedURL
		hrefURL, err := url.Parse(href)
		if err != nil {
			return
		}
		resolvedURL := base.ResolveReference(hrefURL)
		logoURL := resolvedURL.String()

		if logoSet[logoURL] {
			return
		}
		logoSet[logoURL] = true

		sizes := parseSizes(sizesAttr)
		format := getFormatFromURL(logoURL, typ)

		logos = append(logos, types.BrandLogo{
			URL:    logoURL,
			Format: format,
			Sizes:  sizes,
			Mode:   "icon",
		})
	})

	if len(logos) == 0 || (len(logos) == 1 && logos[0].Format == "ico") {
		faviconICO := "https://" + domain + "/favicon.ico"
		if !logoSet[faviconICO] {
			logos = append(logos, types.BrandLogo{
				URL:    faviconICO,
				Format: "ico",
				Mode:   "icon",
			})
		}
	}

	return logos
}

func parseSizes(attr string) []int {
	if attr == "" {
		return nil
	}
	if strings.ToLower(attr) == "any" {
		return nil
	}
	var sizes []int
	for _, p := range strings.Fields(attr) {
		xy := strings.Split(p, "x")
		if len(xy) == 2 {
			if w, err := parseInt(xy[0]); err == nil {
				sizes = append(sizes, w)
			}
		}
	}
	return sizes
}

func getFormatFromURL(srcURL, contentType string) string {
	ext := strings.ToLower(path.Ext(srcURL))
	switch ext {
	case ".svg":
		return "svg"
	case ".ico":
		return "ico"
	case ".webp":
		return "webp"
	case ".avif":
		return "avif"
	case ".png":
		return "png"
	case ".jpg", ".jpeg":
		return "jpeg"
	case ".gif":
		return "gif"
	}

	switch contentType {
	case "image/svg+xml":
		return "svg"
	case "image/x-icon", "image/vnd.microsoft.icon":
		return "ico"
	case "image/webp":
		return "webp"
	case "image/avif":
		return "avif"
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpeg"
	case "image/gif":
		return "gif"
	}

	return "unknown"
}
