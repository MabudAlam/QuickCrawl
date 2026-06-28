package brand

import (
	"strings"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/PuerkitoBio/goquery"
)

func ExtractMetadata(html string, pageURL string) *types.BrandData {
	return ExtractMetadataWithTokens(html, pageURL, nil, nil, "")
}

// ExtractMetadataWithTokens is the brand extraction entry point used by the
// /v1/brand handler. It runs the HTML-based extractors (icons, colors,
// og-image, meta, etc.) and, when present, layers in the fonts + styleguide
// data returned by the in-browser design-token extractor. screenshotColors
// are extracted via vibrant from a full-page screenshot and merged with CSS
// colors when present. pageTitle is the document.title from the browser
// (set dynamically via JS) and is used as fallback when no title is found
// in the HTML.
func ExtractMetadataWithTokens(html string, pageURL string, tokens []byte, screenshotColors []types.BrandColor, pageTitle string) *types.BrandData {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return defaultBrandData(pageURL)
	}

	domain := getDomain(pageURL)

	cssColors := extractColors(doc)
	colors := mergeColors(cssColors, screenshotColors)

	title := extractTitle(doc)
	if title == "" {
		title = pageTitle
	}

	brand := &types.BrandData{
		Domain:          domain,
		Title:           title,
		Description:     extractDescription(doc),
		Colors:          colors,
		Logos:           extractLogos(doc, pageURL),
		Backdrops:       extractBackdrops(doc, pageURL),
		Address:         extractAddress(doc),
		Socials:         extractSocials(doc),
		Links:           extractLinks(doc, domain),
		PrimaryLanguage: detectLanguage(doc),
	}

	if len(tokens) > 0 {
		fonts, sg := ExtractDesignTokens(tokens)
		brand.Fonts = fonts
		brand.Styleguide = sg
	}

	return brand
}

func mergeColors(cssColors, screenshotColors []types.BrandColor) []types.BrandColor {
	if len(screenshotColors) == 0 {
		return cssColors
	}
	if len(cssColors) == 0 {
		return screenshotColors
	}

	seen := make(map[string]bool)
	var result []types.BrandColor

	for _, c := range cssColors {
		lower := strings.ToLower(c.Hex)
		if !seen[lower] {
			seen[lower] = true
			result = append(result, c)
		}
	}
	for _, c := range screenshotColors {
		lower := strings.ToLower(c.Hex)
		if !seen[lower] {
			seen[lower] = true
			result = append(result, c)
		}
	}

	return result
}

func defaultBrandData(pageURL string) *types.BrandData {
	domain := getDomain(pageURL)
	return &types.BrandData{
		Domain:    domain,
		Colors:    []types.BrandColor{},
		Logos:     []types.BrandLogo{},
		Backdrops: []types.BrandBackdrop{},
		Socials:   []types.SocialLink{},
		Links:     &types.BrandLinks{},
	}
}
