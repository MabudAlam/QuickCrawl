package extractor

import (
	"regexp"
	"strings"

	htmldec "github.com/JohannesKaufmann/html-to-markdown"
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
	converter := htmldec.NewConverter("", true, nil)
	md, err := converter.ConvertString(html)
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

func convertIndentedToFencedCode(md string) string {
	var result strings.Builder
	var codeLines []string
	inFenced := false

	for _, line := range strings.Split(md, "\n") {
		if strings.TrimLeft(line, " \t") == "" {
			continue
		}

		trimmed := strings.TrimLeft(line, " \t")

		if strings.HasPrefix(trimmed, "```") {
			if inFenced {
				appendCodeBlock(&result, &codeLines)
				inFenced = false
			} else {
				if len(codeLines) > 0 {
					appendCodeBlock(&result, &codeLines)
				}
				inFenced = true
			}
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		if inFenced {
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		isCodeIndent := strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t")

		if isCodeIndent {
			stripped := strings.TrimLeft(line, " \t")
			codeLines = append(codeLines, stripped)
		} else {
			if len(codeLines) > 0 {
				appendCodeBlock(&result, &codeLines)
			}
			result.WriteString(line)
			result.WriteString("\n")
		}
	}

	if len(codeLines) > 0 {
		appendCodeBlock(&result, &codeLines)
	}

	return result.String()
}

func appendCodeBlock(result *strings.Builder, codeLines *[]string) {
	for len(*codeLines) > 0 && (*codeLines)[len(*codeLines)-1] == "" {
		*codeLines = (*codeLines)[:len(*codeLines)-1]
	}

	if len(*codeLines) == 0 {
		return
	}

	result.WriteString("```\n")
	for _, line := range *codeLines {
		result.WriteString(line)
		result.WriteString("\n")
	}
	result.WriteString("```\n")
	*codeLines = nil
}
