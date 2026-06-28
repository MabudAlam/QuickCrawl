package brand

import (
	"strings"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/PuerkitoBio/goquery"
)

func ExtractMetadata(html string, pageURL string) *types.BrandData {
	return ExtractMetadataWithTokens(html, pageURL, nil)
}

// ExtractMetadataWithTokens is the brand extraction entry point used by the
// /v1/brand handler. It runs the HTML-based extractors (icons, colors,
// og-image, meta, etc.) and, when present, layers in the fonts + styleguide
// data returned by the in-browser design-token extractor.
func ExtractMetadataWithTokens(html string, pageURL string, tokens []byte) *types.BrandData {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return defaultBrandData(pageURL)
	}

	domain := getDomain(pageURL)

	brand := &types.BrandData{
		Domain:          domain,
		Title:           extractTitle(doc),
		Description:     extractDescription(doc),
		Colors:          extractColors(doc),
		Logos:           extractLogos(doc, pageURL),
		Backdrops:       extractBackdrops(doc, pageURL),
		Address:         extractAddress(doc),
		Socials:         extractSocials(doc),
		Links:           extractLinks(doc, domain),
		PrimaryLanguage: detectLanguage(doc),
	}

	if brand.Title == "" {
		brand.Title = brand.Name
	}

	if len(tokens) > 0 {
		fonts, sg := ExtractDesignTokens(tokens)
		brand.Fonts = fonts
		brand.Styleguide = sg
	}

	return brand
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
