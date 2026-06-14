package extractor

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ─── Pipeline Constants ──────────────────────────────────────────────────────

// minUsableMarkdownLen is the minimum trimmed length a markdown candidate
// must reach before we stop trying additional (more expensive) fallback
// extraction strategies. Candidates below this threshold are considered
// insufficient and trigger the next fallback.
// ExtractMetadata extracts page metadata from HTML including title, description,
// author, published/updated timestamps, Open Graph tags, canonical URL, and language.
func ExtractMetadata(html string) ExtractedMetadata {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ExtractedMetadata{}
	}

	var meta ExtractedMetadata

	if title := doc.Find("title").First().Text(); title != "" {
		meta.Title = stringPtr(strings.TrimSpace(title))
	}

	if desc, exists := doc.Find("meta[name=\"description\"]").First().Attr("content"); exists && desc != "" {
		meta.Description = stringPtr(strings.TrimSpace(desc))
	}

	if ogTitle, exists := doc.Find("meta[property=\"og:title\"]").First().Attr("content"); exists && ogTitle != "" {
		meta.OGTitle = stringPtr(strings.TrimSpace(ogTitle))
	}
	if ogDesc, exists := doc.Find("meta[property=\"og:description\"]").First().Attr("content"); exists && ogDesc != "" {
		meta.OGDescription = stringPtr(strings.TrimSpace(ogDesc))
	}
	if ogImg, exists := doc.Find("meta[property=\"og:image\"]").First().Attr("content"); exists && ogImg != "" {
		meta.OGImage = stringPtr(strings.TrimSpace(ogImg))
	}

	if canonical, exists := doc.Find("link[rel=\"canonical\"]").First().Attr("href"); exists && canonical != "" {
		meta.CanonicalURL = stringPtr(strings.TrimSpace(canonical))
	}

	if lang, exists := doc.Find("html").First().Attr("lang"); exists && lang != "" {
		meta.Language = stringPtr(strings.TrimSpace(lang))
	}

	if author, exists := doc.Find("meta[name=\"author\"]").First().Attr("content"); exists && author != "" {
		meta.Author = stringPtr(strings.TrimSpace(author))
	} else if author, exists := doc.Find("meta[property=\"article:author\"]").First().Attr("content"); exists && author != "" {
		meta.Author = stringPtr(strings.TrimSpace(author))
	} else if author, exists := doc.Find("meta[name=\"byl\"]").First().Attr("content"); exists && author != "" {
		meta.Author = stringPtr(strings.TrimSpace(author))
	}

	if pubTime, exists := doc.Find("meta[property=\"article:published_time\"]").First().Attr("content"); exists && pubTime != "" {
		meta.PublishedTime = stringPtr(strings.TrimSpace(pubTime))
	} else if pubTime, exists := doc.Find("meta[name=\"pubdate\"]").First().Attr("content"); exists && pubTime != "" {
		meta.PublishedTime = stringPtr(strings.TrimSpace(pubTime))
	} else if pubTime, exists := doc.Find("meta[name=\"date\"]").First().Attr("content"); exists && pubTime != "" {
		meta.PublishedTime = stringPtr(strings.TrimSpace(pubTime))
	} else if pubTime, exists := doc.Find("meta[itemprop=\"datePublished\"]").First().Attr("content"); exists && pubTime != "" {
		meta.PublishedTime = stringPtr(strings.TrimSpace(pubTime))
	}

	if updTime, exists := doc.Find("meta[property=\"article:modified_time\"]").First().Attr("content"); exists && updTime != "" {
		meta.ModifiedTime = stringPtr(strings.TrimSpace(updTime))
	} else if updTime, exists := doc.Find("meta[name=\"lastmod\"]").First().Attr("content"); exists && updTime != "" {
		meta.ModifiedTime = stringPtr(strings.TrimSpace(updTime))
	} else if updTime, exists := doc.Find("meta[itemprop=\"dateModified\"]").First().Attr("content"); exists && updTime != "" {
		meta.ModifiedTime = stringPtr(strings.TrimSpace(updTime))
	}

	return meta
}

