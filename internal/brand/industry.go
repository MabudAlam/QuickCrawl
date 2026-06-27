package brand

import (
	"strings"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/PuerkitoBio/goquery"
)

var industryKeywords = map[string]string{
	"technology":              "Technology",
	"software":               "Technology",
	"artificial intelligence": "Artificial Intelligence & Machine Learning",
	"machine learning":        "Artificial Intelligence & Machine Learning",
	"ai":                     "Artificial Intelligence & Machine Learning",
	"fintech":                 "Fintech",
	"financial technology":    "Fintech",
	"healthcare":             "Healthcare",
	"medical":                "Healthcare",
	"ecommerce":              "E-commerce",
	"e-commerce":             "E-commerce",
	"saas":                   "SaaS",
	"cloud":                  "Cloud Computing",
	"cybersecurity":         "Cybersecurity",
	"security":               "Cybersecurity",
	"blockchain":              "Blockchain",
	"crypto":                 "Cryptocurrency",
	"cryptocurrency":          "Cryptocurrency",
	"gaming":                 "Gaming",
	"game":                   "Gaming",
	"media":                  "Media & Entertainment",
	"marketing":              "Marketing & Advertising",
	"consulting":             "Consulting",
	"education":              "Education",
	"edtech":                  "Education",
	"health":                 "Health & Wellness",
	"fitness":                "Health & Wellness",
	"food":                   "Food & Beverage",
	"travel":                 "Travel & Hospitality",
	"real estate":            "Real Estate",
	"property":               "Real Estate",
	"automotive":             "Automotive",
	"cars":                   "Automotive",
	"energy":                 "Energy",
	"manufacturing":          "Manufacturing",
	"retail":                 "Retail",
	"telecom":                "Telecommunications",
	"telecommunications":     "Telecommunications",
}

func extractIndustries(doc *goquery.Document) *types.IndustryMap {
	foundIndustries := make(map[string]string)
	text := doc.Text()
	lowerText := strings.ToLower(text)

	for keyword, industry := range industryKeywords {
		if strings.Contains(lowerText, keyword) {
			foundIndustries[industry] = industry
		}
	}

	doc.Find("meta").Each(func(i int, s *goquery.Selection) {
		name, _ := s.Attr("name")
		if name == "keywords" || name == "subject" {
			content, _ := s.Attr("content")
			keywords := strings.Split(strings.ToLower(content), ",")
			for _, kw := range keywords {
				kw = strings.TrimSpace(kw)
				for keyword, industry := range industryKeywords {
					if strings.Contains(kw, keyword) {
						foundIndustries[industry] = industry
					}
				}
			}
		}
	})

	var eic []types.Industry
	for industry := range foundIndustries {
		eic = append(eic, types.Industry{
			Industry:   industry,
			Subindustry: industry,
		})
	}

	if len(eic) == 0 {
		return nil
	}

	return &types.IndustryMap{
		EIC: eic,
	}
}
