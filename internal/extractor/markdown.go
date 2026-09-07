package extractor

import (
	"regexp"

	"github.com/JohannesKaufmann/html-to-markdown/v2"
)

// Post-processing regexes for markdown artifact cleanup.
var (
	// pilcrowRe removes ¶ (pilcrow sign) added by some converters.
	pilcrowRe = regexp.MustCompile("\u00b6")

	// sectionSignRe removes § (section sign) with preceding space.
	sectionSignRe = regexp.MustCompile(" \u00a7")

	// emptyAnchorRe removes empty anchor links like [¶]() or []().
	emptyAnchorRe = regexp.MustCompile(`\[¶?\]\(#[^)]*\)`)

	// dataUriRe removes markdown images with data: URIs.
	dataUriRe = regexp.MustCompile(`!\[[^\]]*\]\(data:[^)]*\)`)

	// markdownImgRe removes all markdown images `![alt](url)`.
	markdownImgRe = regexp.MustCompile(`!\[([^\]]*)\]\([^)]+\)`)

	// emptyImgRe removes empty image markdown `![](/)`.
	emptyImgRe = regexp.MustCompile(`!\[\]\(/\)`)

	// emptyImgAltRe removes image markdown with alt but no URL `![alt]()`.
	emptyImgAltRe = regexp.MustCompile(`!\[[^\]]*\]\(\)`)
)

// HTMLToMarkdown converts HTML content to Markdown format using the
// html-to-markdown library. It then runs post-processing to clean up
// common artifacts: empty anchors, pilcrow signs (¶), section signs (§),
// data URI images, and all markdown images (since images are extracted
// separately via ExtractImageURLs).
func HTMLToMarkdown(html string) string {
	md, err := htmltomarkdown.ConvertString(html)
	if err != nil {
		return ""
	}

	return postProcessMarkdown(md)
}

// postProcessMarkdown cleans up artifacts left by the markdown converter.
func postProcessMarkdown(md string) string {
	result := md
	result = emptyAnchorRe.ReplaceAllString(result, "")
	result = pilcrowRe.ReplaceAllString(result, "")
	result = sectionSignRe.ReplaceAllString(result, "")
	result = dataUriRe.ReplaceAllString(result, "")
	result = emptyImgRe.ReplaceAllString(result, "")
	result = emptyImgAltRe.ReplaceAllString(result, "")
	result = markdownImgRe.ReplaceAllString(result, "")
	return result
}

// FilterMarkdownImages removes all markdown images `![alt](url)` from the markdown text.
func FilterMarkdownImages(md string) string {
	return markdownImgRe.ReplaceAllString(md, "")
}
