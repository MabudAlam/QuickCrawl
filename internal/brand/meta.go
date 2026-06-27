package brand

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func extractTitle(doc *goquery.Document) string {
	var title string
	doc.Find("meta").Each(func(i int, s *goquery.Selection) {
		if title != "" {
			return
		}
		property, _ := s.Attr("property")
		if property == "og:title" || property == "og:site_name" {
			title, _ = s.Attr("content")
		}
	})
	if title == "" {
		doc.Find("title").Each(func(i int, s *goquery.Selection) {
			if title == "" {
				title = s.Text()
			}
		})
	}
	return strings.TrimSpace(title)
}

func extractDescription(doc *goquery.Document) string {
	var description string
	doc.Find("meta").Each(func(i int, s *goquery.Selection) {
		if description != "" {
			return
		}
		property, _ := s.Attr("property")
		name, _ := s.Attr("name")
		if property == "og:description" || name == "description" {
			description, _ = s.Attr("content")
		}
	})
	return strings.TrimSpace(description)
}

func extractTagline(doc *goquery.Document) string {
	var tagline string
	doc.Find("meta").Each(func(i int, s *goquery.Selection) {
		if tagline != "" {
			return
		}
		property, _ := s.Attr("property")
		if property == "og:description" {
			tagline, _ = s.Attr("content")
		}
	})
	return strings.TrimSpace(tagline)
}
