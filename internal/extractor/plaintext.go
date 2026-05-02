package extractor

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// HTMLToPlaintext converts HTML content to plain text by extracting visible
// text and collapsing whitespace. It removes all HTML tags and normalizes
// spacing between words and paragraphs.
func HTMLToPlaintext(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ""
	}

	text := doc.Text()

	var result strings.Builder
	result.Grow(len(text))
	prevWasSpace := true

	for _, ch := range text {
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			if !prevWasSpace {
				result.WriteByte(' ')
				prevWasSpace = true
			}
		} else {
			result.WriteRune(ch)
			prevWasSpace = false
		}
	}

	return strings.TrimSpace(result.String())
}
