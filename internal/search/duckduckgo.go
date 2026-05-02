package search

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type TextResult = struct {
	Title string
	Href  string
	Body  string
}

type BaseSearchEngine struct {
	searchURL     string
	searchMethod  string
	itemsXpath    string
	elementsXpath map[string]string
}

func (e *BaseSearchEngine) buildPayload(query, region, safesearch, timelimit string) map[string]string {
	payload := map[string]string{"q": query, "b": "", "kl": region}
	if timelimit != "" {
		payload["df"] = timelimit
	}
	return payload
}

type Duckduckgo struct {
	BaseSearchEngine
}

func New() *Duckduckgo {
	return &Duckduckgo{
		BaseSearchEngine: BaseSearchEngine{
			searchURL:    "https://html.duckduckgo.com/html/",
			searchMethod: "POST",
			itemsXpath:   "//div[contains(@class, 'body')]",
			elementsXpath: map[string]string{
				"title": ".//h2//text()",
				"href":  "./a/@href",
				"body":  "./a//text()",
			},
		},
	}
}

func (d *Duckduckgo) Search(query, region, safesearch, timelimit string) ([]TextResult, error) {
	payload := d.buildPayload(query, region, safesearch, timelimit)

	body, err := d.request(payload)
	if err != nil {
		return nil, err
	}

	results := d.extractResults(body)

	return results, nil
}

func (d *Duckduckgo) request(payload map[string]string) (string, error) {
	formData := url.Values{}
	for k, v := range payload {
		formData.Set(k, v)
	}

	req, err := http.NewRequest("POST", d.searchURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (d *Duckduckgo) extractResults(htmlText string) []TextResult {
	var results []TextResult

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlText))
	if err != nil {
		return results
	}

	doc.Find(".result").Each(func(i int, s *goquery.Selection) {
		resultLink := s.Find("a.result__a")
		title := strings.TrimSpace(resultLink.Text())
		href, _ := resultLink.Attr("href")

		if title == "" || href == "" || strings.Contains(href, "duckduckgo.com/y.js") {
			return
		}

		snippetLink := s.Find("a.result__snippet")
		body := strings.TrimSpace(snippetLink.Text())

		results = append(results, TextResult{
			Title: title,
			Href:  href,
			Body:  body,
		})
	})

	return results
}