package brand

import (
	"regexp"
	"strings"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/PuerkitoBio/goquery"
)

func extractAddress(doc *goquery.Document) *types.BrandAddress {
	address := &types.BrandAddress{}

	text := doc.Text()
	lowerText := strings.ToLower(text)

	if !containsAddressKeyword(lowerText) {
		return nil
	}

	cityRegex := regexp.MustCompile(`(?i)(?:city|town)[:\s]+([A-Za-z\s]+?)(?:,|\n|$)`)
	countryRegex := regexp.MustCompile(`(?i)(?:country|nation)[:\s]+([A-Za-z\s]+?)(?:,|\n|$)`)
	postalRegex := regexp.MustCompile(`(?i)(?:postal|zip|pin)[:\s]*([0-9A-Za-z\s-]+?)(?:,|\n|$)`)
	stateRegex := regexp.MustCompile(`(?i)(?:state|province)[:\s]+([A-Za-z\s]+?)(?:,|\n|$)`)

	if cityMatch := cityRegex.FindStringSubmatch(text); len(cityMatch) > 1 {
		address.City = strings.TrimSpace(cityMatch[1])
	}
	if countryMatch := countryRegex.FindStringSubmatch(text); len(countryMatch) > 1 {
		address.Country = strings.TrimSpace(countryMatch[1])
	}
	if postalMatch := postalRegex.FindStringSubmatch(text); len(postalMatch) > 1 {
		address.PostalCode = strings.TrimSpace(postalMatch[1])
	}
	if stateMatch := stateRegex.FindStringSubmatch(text); len(stateMatch) > 1 {
		address.StateProvince = strings.TrimSpace(stateMatch[1])
	}

	if address.City != "" || address.Country != "" {
		return address
	}
	return nil
}

func containsAddressKeyword(text string) bool {
	keywords := []string{"address", "footer", "contact", "location"}
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

func extractEmail(doc *goquery.Document) string {
	emailRegex := regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	matches := emailRegex.FindAllString(doc.Text(), -1)
	for _, match := range matches {
		if !strings.Contains(match, "example") && !strings.Contains(match, "test") {
			return match
		}
	}
	return ""
}

func extractLinks(doc *goquery.Document, domain string) *types.BrandLinks {
	links := &types.BrandLinks{}

	linkPatterns := map[string]*string{
		"blog":    &links.Blog,
		"careers": &links.Careers,
		"contact": &links.Contact,
		"pricing": &links.Pricing,
		"privacy": &links.Privacy,
		"terms":   &links.Terms,
		"login":   &links.Login,
		"signup":  &links.Signup,
	}

	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		text := strings.ToLower(s.Text())
		hrefLower := strings.ToLower(href)

		for keyword, ptr := range linkPatterns {
			if *ptr == "" && (strings.Contains(text, keyword) || strings.Contains(hrefLower, keyword)) {
				*ptr = resolveURL(href, "https://"+domain)
			}
		}
	})

	return links
}

func detectLanguage(doc *goquery.Document) string {
	htmlLang := doc.Find("html").AttrOr("lang", "")
	if htmlLang != "" {
		return strings.Split(htmlLang, "-")[0]
	}

	metaLang := ""
	doc.Find("meta").Each(func(i int, s *goquery.Selection) {
		if metaLang != "" {
			return
		}
		httpEquiv, _ := s.Attr("http-equiv")
		if strings.ToLower(httpEquiv) == "content-language" {
			metaLang, _ = s.Attr("content")
		}
	})
	if metaLang != "" {
		return strings.Split(metaLang, "-")[0]
	}

	return "english"
}
