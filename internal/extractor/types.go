// Package extractor provides content extraction from HTML.
// It converts raw HTML into various output formats (Markdown, plain text, etc.)
// and can extract metadata and filter results.
package extractor

import (
	"github.com/MabudAlam/quickcrawl/internal/types"
)

// ExtractOptions contains all parameters for content extraction.
// All fields are optional except RawHTML or RawBytes must be provided.
type ExtractOptions struct {
	// RawHTML is the raw HTML content to extract from
	RawHTML string

	// RawBytes contains raw bytes for PDF processing
	RawBytes []byte

	// SourceURL is the URL the content came from (used for link resolution)
	SourceURL string

	// StatusCode is the HTTP status code from the fetch
	StatusCode int

	// RenderedMode indicates how the content was rendered (e.g., "http", "pdf")
	RenderedMode *string


	// Formats specifies which output formats to generate
	Formats []types.OutputFormat

	// IncludeTags filters to only include these HTML tags
	IncludeTags []string

	// ExcludeTags removes these HTML tags from content
	ExcludeTags []string

	// CSSSelector extracts content from elements matching this selector
	CSSSelector *string
}

// ExtractedMetadata contains page metadata extracted from HTML.
// All fields are pointers to allow distinguishing between empty and unset values.
type ExtractedMetadata struct {
	// Title is the page title from <title> tag
	Title *string

	// Description is the meta description content
	Description *string

	// Language is the page language from html lang attribute
	Language *string

	// OGTitle is the Open Graph title
	OGTitle *string

	// OGDescription is the Open Graph description
	OGDescription *string

	// OGImage is the Open Graph image URL
	OGImage *string

	// CanonicalURL is the canonical URL from link rel="canonical"
	CanonicalURL *string
}

// ExtractedHTML contains the extracted HTML content along with its metadata.
type ExtractedHTML struct {
	// Title is the article title extracted by readability
	Title string
	// Excerpt is the article excerpt/summary extracted by readability
	Excerpt string
	// Content is the main article HTML content
	Content string
}
