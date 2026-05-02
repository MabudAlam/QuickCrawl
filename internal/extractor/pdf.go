package extractor

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/ledongthuc/pdf"
)

var pdfTextRe = regexp.MustCompile(`\(([^()]*)\)\s*T[Jj]`)
var pdfStreamRe = regexp.MustCompile(`stream\s*\(\s*([^\)]+)\s*\)`)
var pdfObjRe = regexp.MustCompile(`(\d+)\s+\d+\s+obj`)

const (
	pdfTypeTextBased  = iota
	pdfTypeImageBased
	pdfTypeScanned
)

type PDFResult struct {
	Text      string
	PDFType   int
	Title     string
	HasText   bool
	PageCount int
}

func ExtractPDFText(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}

	reader := bytes.NewReader(raw)
	r, err := pdf.NewReader(reader, int64(len(raw)))
	if err != nil {
		return extractPDFTextFallback(raw)
	}

	var buf bytes.Buffer
	numPages := r.NumPage()
	for i := 1; i <= numPages; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		buf.WriteString(text)
		buf.WriteString("\n")
	}

	result := strings.TrimSpace(buf.String())
	if result == "" {
		return extractPDFTextFallback(raw)
	}
	return result
}

func extractPDFTextFallback(raw []byte) string {
	s := string(raw)
	matches := pdfTextRe.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return ""
	}

	var parts []string
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		part := unescapePDFString(m[1])
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "\n")
}

func unescapePDFString(s string) string {
	var buf bytes.Buffer
	buf.Grow(len(s))
	escaped := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escaped {
			switch ch {
			case 'n':
				buf.WriteByte('\n')
			case 'r':
				buf.WriteByte('\r')
			case 't':
				buf.WriteByte('\t')
			case 'b':
				buf.WriteByte('\b')
			case 'f':
				buf.WriteByte('\f')
			case '\\', '(', ')':
				buf.WriteByte(ch)
			default:
				buf.WriteByte(ch)
			}
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		buf.WriteByte(ch)
	}
	return buf.String()
}

func detectPDFType(raw []byte) (pdfType int, hasText bool) {
	text := ExtractPDFText(raw)

	if len(text) < 50 {
		hasText = false
		pdfType = pdfTypeScanned
		return
	}

	hasText = true

	imageIndicators := []string{
		"/Type /Page",
		"/XObject",
		"/Image",
		"/Subtype /Image",
		"Do",
	}

	imageCount := 0
	lowerRaw := strings.ToLower(string(raw))
	for _, indicator := range imageIndicators {
		imageCount += strings.Count(lowerRaw, indicator)
	}

	if imageCount > 10 {
		pdfType = pdfTypeImageBased
	} else {
		pdfType = pdfTypeTextBased
	}

	return
}

func estimatePageCount(raw []byte) int {
	count := strings.Count(string(raw), "/Type /Page")
	if count == 0 {
		count = 1
	}
	return count
}

func extractPDFMetadata(raw []byte) (title string) {
	re := regexp.MustCompile(`(?i)/Title\s*\(([^)]*)\)`)
	matches := re.FindStringSubmatch(string(raw))
	if len(matches) > 1 {
		title = unescapePDFString(matches[1])
	}
	return
}

func ProcessPDF(raw []byte) PDFResult {
	pdfType, hasText := detectPDFType(raw)
	title := extractPDFMetadata(raw)
	pageCount := estimatePageCount(raw)

	text := ExtractPDFText(raw)

	return PDFResult{
		Text:      text,
		PDFType:   pdfType,
		Title:     title,
		HasText:   hasText,
		PageCount: pageCount,
	}
}
