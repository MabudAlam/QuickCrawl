package brand

import (
	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/PuerkitoBio/goquery"
)

func extractBackdrops(doc *goquery.Document, pageURL string) []types.BrandBackdrop {
	var backdrops []types.BrandBackdrop

	ogImage := ""
	doc.Find("meta").Each(func(i int, s *goquery.Selection) {
		if ogImage != "" {
			return
		}
		property, _ := s.Attr("property")
		if property == "og:image" {
			ogImage, _ = s.Attr("content")
		}
	})

	if ogImage != "" {
		backdrops = append(backdrops, types.BrandBackdrop{
			URL: resolveURL(ogImage, pageURL),
		})
	}

	return backdrops
}
