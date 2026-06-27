package brand

import (
	"encoding/json"

	"github.com/MabudAlam/quickcrawl/internal/types"
)

// ExtractDesignTokens parses the JSON payload returned by the in-browser
// brand extractor. The payload is a single object with two top-level keys:
//
//	{ "fonts": { ... }, "styleguide": { ... } }
//
// Missing keys are tolerated — the returned values may have either or both
// fields nil. Callers should check before using.
func ExtractDesignTokens(payload json.RawMessage) (*types.BrandFonts, *types.BrandStyleguide) {
	if len(payload) == 0 {
		return nil, nil
	}

	var wrap struct {
		Fonts      json.RawMessage `json:"fonts"`
		Styleguide json.RawMessage `json:"styleguide"`
	}
	if err := json.Unmarshal(payload, &wrap); err != nil {
		return nil, nil
	}

	fonts := parseFonts(wrap.Fonts)
	styleguide := parseStyleguide(wrap.Styleguide)
	return fonts, styleguide
}

func parseFonts(raw json.RawMessage) *types.BrandFonts {
	if len(raw) == 0 {
		return nil
	}
	// The browser payload nests under a "fonts" object: { fonts: [...], fontLinks: {...} }
	// or under a "fonts" array directly. Tolerate both.
	var asArray []types.BrandFont
	if err := json.Unmarshal(raw, &asArray); err == nil && asArray != nil {
		return &types.BrandFonts{Fonts: asArray}
	}
	var asObject struct {
		Fonts     []types.BrandFont           `json:"fonts"`
		FontLinks map[string]types.BrandFontLink `json:"fontLinks"`
	}
	if err := json.Unmarshal(raw, &asObject); err != nil {
		return nil
	}
	if asObject.Fonts == nil && asObject.FontLinks == nil {
		return nil
	}
	return &types.BrandFonts{
		Fonts:     asObject.Fonts,
		FontLinks: asObject.FontLinks,
	}
}

func parseStyleguide(raw json.RawMessage) *types.BrandStyleguide {
	if len(raw) == 0 {
		return nil
	}
	var sg types.BrandStyleguide
	if err := json.Unmarshal(raw, &sg); err != nil {
		return nil
	}
	if sg.Mode == "" && sg.Colors.Accent == "" && sg.Typography.P.FontFamily == "" {
		return nil
	}
	return &sg
}