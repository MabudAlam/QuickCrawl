package extractor

import (
	"regexp"
	"strings"

	htmldec "github.com/JohannesKaufmann/html-to-markdown"
)

var (
	pilcrowRe     = regexp.MustCompile("\u00b6")
	sectionSignRe = regexp.MustCompile(" \u00a7")
	emptyAnchorRe = regexp.MustCompile(`\[¶?\]\(#[^)]*\)`)
	dataUriRe     = regexp.MustCompile(`!\[[^\]]*\]\(data:[^)]*\)`)
)

// HTMLToMarkdown converts HTML content to Markdown format using the
// html-to-markdown library. It also cleans up common artifacts like
// empty anchors, pilcrow signs (¶), section signs (§), and data URI images.
func HTMLToMarkdown(html string) string {
	converter := htmldec.NewConverter("", true, nil)
	md, err := converter.ConvertString(html)
	if err != nil {
		return ""
	}

	result := md
	result = emptyAnchorRe.ReplaceAllString(result, "")
	result = pilcrowRe.ReplaceAllString(result, "")
	result = sectionSignRe.ReplaceAllString(result, "")
	result = dataUriRe.ReplaceAllString(result, "")

	return result
}

// convertIndentedToFencedCode converts indented code blocks to fenced code
// blocks (triple backticks) in Markdown text. This is useful for converting
// legacy Markdown formats to standard fenced code blocks.
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

// appendCodeBlock appends a fenced code block to the result builder using
// the collected code lines. It trims trailing empty lines before appending.
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
