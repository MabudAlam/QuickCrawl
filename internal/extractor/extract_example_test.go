package extractor

import (
	"testing"

	quickcrawlcore "github.com/MabudAlam/quickcrawl/internal/types"
)

func TestExtractExampleComHTML(t *testing.T) {
	html := `<!doctype html><html lang="en"><head><title>Example Domain</title><meta name="viewport" content="width=device-width, initial-scale=1"><style>body{background:#eee;width:60vw;margin:15vh auto;font-family:system-ui,sans-serif}h1{font-size:1.5em}div{opacity:0.8}a:link,a:visited{color:#348}</style></head><body><div><h1>Example Domain</h1><p>This domain is for use in documentation examples without needing permission. Avoid use in operations.</p><p><a href="https://iana.org/domains/example">Learn more</a></p></div></body></html>`

	data := Extract(ExtractOptions{
		RawHTML:      html,
		SourceURL:    "https://example.com",
		StatusCode:   200,
		RenderedMode: nil,
		Formats:      []quickcrawlcore.OutputFormat{quickcrawlcore.FormatMarkdown},
	})

	if data.Markdown == nil || *data.Markdown == "" {
		t.Fatal("expected markdown output")
	}
}
