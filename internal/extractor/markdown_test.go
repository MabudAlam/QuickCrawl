package extractor

import (
	"strings"
	"testing"
)

func TestHTMLToMarkdownConvertsHeadings(t *testing.T) {
	html := `<h1>Heading One</h1><h2>Heading Two</h2>`
	md := HTMLToMarkdown(html)
	if !strings.Contains(md, "# Heading One") {
		t.Error("Expected h1 to convert to markdown # heading")
	}
}

func TestHTMLToMarkdownConvertsBold(t *testing.T) {
	html := `<p>Text with <strong>bold</strong> word.</p>`
	md := HTMLToMarkdown(html)
	lower := strings.ToLower(md)
	if !strings.Contains(lower, "bold") {
		t.Error("Expected bold text to be preserved")
	}
}

func TestHTMLToMarkdownConvertsItalic(t *testing.T) {
	html := `<p>Text with <em>italic</em> word.</p>`
	md := HTMLToMarkdown(html)
	lower := strings.ToLower(md)
	if !strings.Contains(lower, "italic") {
		t.Error("Expected italic text to be preserved")
	}
}

func TestHTMLToMarkdownLinks(t *testing.T) {
	html := `<p>Link to <a href="https://example.com">Example</a>.</p>`
	md := HTMLToMarkdown(html)
	if !strings.Contains(md, "[Example]") {
		t.Error("Expected link to convert to markdown")
	}
}

func TestHTMLToMarkdownLists(t *testing.T) {
	html := `<ul><li>Item 1</li><li>Item 2</li></ul>`
	md := HTMLToMarkdown(html)
	if !strings.Contains(md, "- Item 1") {
		t.Error("Expected list to convert to markdown")
	}
}

func TestHTMLToMarkdownCodeBlock(t *testing.T) {
	html := `<pre><code>code here</code></pre>`
	md := HTMLToMarkdown(html)
	if !strings.Contains(md, "```") {
		t.Error("Expected code block to convert to fenced code")
	}
}

func TestHTMLToMarkdownRemovesEmptyAnchors(t *testing.T) {
	html := `<p><a href="#section">¶</a></p>`
	md := HTMLToMarkdown(html)
	if strings.Contains(md, "[¶]") {
		t.Error("Expected empty anchor markers to be removed")
	}
}

func TestHTMLToMarkdownRemovesPilcrow(t *testing.T) {
	html := `<p>Text with ¶ pilcrow.</p>`
	md := HTMLToMarkdown(html)
	if strings.Contains(md, "¶") {
		t.Error("Expected pilcrow to be removed")
	}
}

func TestHTMLToMarkdownRemovesSectionSign(t *testing.T) {
	html := `<p>Section § sign.</p>`
	md := HTMLToMarkdown(html)
	if strings.Contains(md, "§") {
		t.Error("Expected section sign to be removed")
	}
}

func TestHTMLToPlaintextStripsTags(t *testing.T) {
	html := `<p>Hello <strong>World</strong></p>`
	text := HTMLToPlaintext(html)
	if strings.Contains(text, "<") {
		t.Error("Expected HTML tags to be stripped")
	}
}

func TestHTMLToPlaintextPreservesText(t *testing.T) {
	html := `<h1>Title</h1><p>Paragraph text.</p>`
	text := HTMLToPlaintext(html)
	if !strings.Contains(text, "Title") {
		t.Error("Expected title text to be preserved")
	}
	if !strings.Contains(text, "Paragraph") {
		t.Error("Expected paragraph text to be preserved")
	}
}

func TestHTMLToPlaintextHandlesLinks(t *testing.T) {
	html := `<p>Link to <a href="https://example.com">Example</a>.</p>`
	text := HTMLToPlaintext(html)
	if strings.Contains(text, "[") || strings.Contains(text, "]") {
		t.Error("Expected link brackets to be removed in plaintext")
	}
}

func TestConvertIndentedToFencedCode(t *testing.T) {
	md := `Paragraph before

    code line one
    code line two
    code line three

Paragraph after`
	result := convertIndentedToFencedCode(md)
	if !strings.Contains(result, "```") {
		t.Error("Expected indented code to become fenced")
	}
}

func TestConvertIndentedToFencedCodeMultipleBlocks(t *testing.T) {
	md := `First paragraph

    first code block
    more first

Second paragraph

    second code block
    more second`
	result := convertIndentedToFencedCode(md)
	count := strings.Count(result, "```")
	if count < 2 {
		t.Errorf("Expected at least 2 code blocks, got %d", count/2)
	}
}

func TestConvertIndentedToFencedCodeNoChange(t *testing.T) {
	md := "Regular paragraph without indented code."
	result := convertIndentedToFencedCode(md)
	if !strings.Contains(result, md) {
		t.Error("Expected original text to be preserved")
	}
}

func TestConvertIndentedToFencedCodeShortLinesIgnored(t *testing.T) {
	md := "Paragraph\n    verylongword"
	result := convertIndentedToFencedCode(md)
	if !strings.Contains(result, "```") {
		t.Error("Expected valid code block to be converted")
	}
}
