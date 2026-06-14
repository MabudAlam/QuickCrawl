package core

import (
	"strings"

	"github.com/MabudAlam/quickcrawl/internal/extractor"
	"github.com/MabudAlam/quickcrawl/internal/types"
)

type Extractor struct{}

func NewExtractor() *Extractor {
	return &Extractor{}
}

func (e *Extractor) Extract(opts extractor.ExtractOptions) *types.ScrapeData {
	internalOpts := extractor.ExtractOptions{
		RawHTML:       opts.RawHTML,
		RawBytes:      opts.RawBytes,
		SourceURL:     opts.SourceURL,
		StatusCode:    opts.StatusCode,
		RenderedMode:  opts.RenderedMode,
		Formats:       opts.Formats,
		IncludeTags:   opts.IncludeTags,
		ExcludeTags:   opts.ExcludeTags,
		CSSSelector:   opts.CSSSelector,
		ExtractorType: extractor.ExtractorTrafilatura,
	}

	data := extractor.Extract(internalOpts)
	if data == nil {
		return &types.ScrapeData{
			Metadata: types.PageMetadata{
				SourceURL:  opts.SourceURL,
				StatusCode: uint16(opts.StatusCode),
			},
		}
	}

	return &types.ScrapeData{
		Markdown:   data.Markdown,
		HTML:       data.HTML,
		RawHTML:    data.RawHTML,
		PlainText:  data.PlainText,
		Links:      data.Links,
		ImageLinks: data.ImageLinks,
		JSON:       data.JSON,
		Warning:    data.Warning,
		Metadata: types.PageMetadata{
			Title:         data.Metadata.Title,
			Description:   data.Metadata.Description,
			OGTitle:       data.Metadata.OGTitle,
			OGDescription: data.Metadata.OGDescription,
			OGImage:       data.Metadata.OGImage,
			CanonicalURL:  data.Metadata.CanonicalURL,
			SourceURL:     data.Metadata.SourceURL,
			Language:      data.Metadata.Language,
			StatusCode:    data.Metadata.StatusCode,
			RenderedMode:  data.Metadata.RenderedMode,
		},
	}
}

func containsFormat(formats []types.OutputFormat, target types.OutputFormat) bool {
	for _, f := range formats {
		if f == target {
			return true
		}
	}
	return false
}

func includesJSONFormat(formats []types.OutputFormat) bool {
	return containsFormat(formats, types.FormatJson)
}

func buildFetchWarning(result *FetchResult) *string {
	if result.Warning != nil {
		return result.Warning
	}
	if result.StatusCode >= 400 {
		w := strings.TrimSpace(result.HTML)
		if len(w) > 100 {
			w = w[:100]
		}
		return &w
	}
	return detectBlockInterstitial(result.HTML)
}

func detectBlockInterstitial(html string) *string {
	if html == "" {
		return nil
	}
	lower := strings.ToLower(html)
	markers := []string{
		"just a moment",
		"attention required",
		"cf-browser-verification",
		"cf-challenge",
		"captcha",
		"access denied",
	}
	for _, m := range markers {
		if strings.Contains(lower, m) {
			msg := "blocked by anti-bot protection"
			return &msg
		}
	}
	return nil
}
