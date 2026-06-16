package types

import (
	"testing"
)

func TestSearchRequest_Validate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(r *SearchRequest)
		wantErr bool
	}{
		{
			name:   "valid minimal",
			mutate: func(r *SearchRequest) {},
		},
		{
			name:   "valid timeRange day",
			mutate: func(r *SearchRequest) { r.TimeRange = "day" },
		},
		{
			name:   "valid timeRange year",
			mutate: func(r *SearchRequest) { r.TimeRange = "year" },
		},
		{
			name:    "invalid timeRange",
			mutate:  func(r *SearchRequest) { r.TimeRange = "hour" },
			wantErr: true,
		},
		{
			name:   "valid categories",
			mutate: func(r *SearchRequest) { r.Categories = "general,news" },
		},
		{
			name:   "valid category with spaces",
			mutate: func(r *SearchRequest) { r.Categories = "social media" },
		},
		{
			name:    "invalid category",
			mutate:  func(r *SearchRequest) { r.Categories = "general,bogus" },
			wantErr: true,
		},
		{
			name:   "valid page 1",
			mutate: func(r *SearchRequest) { r.Page = 1 },
		},
		{
			name:    "negative page",
			mutate:  func(r *SearchRequest) { r.Page = -5 },
			wantErr: true,
		},
		{
			name:    "page too large",
			mutate:  func(r *SearchRequest) { r.Page = 10000 },
			wantErr: true,
		},
		{
			name:   "valid renderMode browser",
			mutate: func(r *SearchRequest) { mode := RenderModeBrowser; r.RenderMode = &mode },
		},
		{
			name:    "invalid renderMode",
			mutate:  func(r *SearchRequest) { mode := RenderMode("smoke"); r.RenderMode = &mode },
			wantErr: true,
		},
		{
			name:    "empty query",
			mutate:  func(r *SearchRequest) { r.Query = "   " },
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := SearchRequest{Query: "hello"}
			r.Defaults()
			tc.mutate(&r)
			err := r.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestScrapeRequest_Validate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(r *ScrapeRequest)
		wantErr bool
	}{
		{name: "valid minimal", mutate: func(r *ScrapeRequest) {}},
		{name: "missing url", mutate: func(r *ScrapeRequest) { r.URL = "" }, wantErr: true},
		{name: "whitespace url", mutate: func(r *ScrapeRequest) { r.URL = "   " }, wantErr: true},
		{name: "ftp scheme", mutate: func(r *ScrapeRequest) { r.URL = "ftp://example.com" }, wantErr: true},
		{name: "valid renderMode auto", mutate: func(r *ScrapeRequest) { m := RenderModeAuto; r.RenderMode = &m }},
		{name: "valid renderMode http", mutate: func(r *ScrapeRequest) { m := RenderModeHTTP; r.RenderMode = &m }},
		{name: "invalid renderMode", mutate: func(r *ScrapeRequest) { m := RenderMode("turbo"); r.RenderMode = &m }, wantErr: true},
		{name: "waitFor zero ok", mutate: func(r *ScrapeRequest) { v := int64(0); r.WaitFor = &v }},
		{name: "waitFor too large", mutate: func(r *ScrapeRequest) { v := int64(200000); r.WaitFor = &v }, wantErr: true},
		{name: "negative waitFor", mutate: func(r *ScrapeRequest) { v := int64(-1); r.WaitFor = &v }, wantErr: true},
		{name: "negative ttl", mutate: func(r *ScrapeRequest) { v := int64(-1); r.TTL = &v }, wantErr: true},
		{name: "ttl zero ok", mutate: func(r *ScrapeRequest) { v := int64(0); r.TTL = &v }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := ScrapeRequest{URL: "https://example.com"}
			r.Defaults()
			tc.mutate(&r)
			err := r.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCrawlRequest_Validate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(r *CrawlRequest)
		wantErr bool
	}{
		{name: "valid minimal", mutate: func(r *CrawlRequest) {}},
		{name: "missing url", mutate: func(r *CrawlRequest) { r.URL = "" }, wantErr: true},
		{name: "bad url", mutate: func(r *CrawlRequest) { r.URL = "not a url" }, wantErr: true},
		{name: "maxDepth nil ok", mutate: func(r *CrawlRequest) { r.MaxDepth = nil }},
		{name: "maxDepth 0 ok", mutate: func(r *CrawlRequest) { v := uint32(0); r.MaxDepth = &v }},
		{name: "maxDepth 100 ok", mutate: func(r *CrawlRequest) { v := uint32(100); r.MaxDepth = &v }},
		{name: "maxDepth too large", mutate: func(r *CrawlRequest) { v := uint32(101); r.MaxDepth = &v }, wantErr: true},
		{name: "maxPages 1 ok", mutate: func(r *CrawlRequest) { v := uint32(1); r.MaxPages = &v }},
		{name: "maxPages 100 ok", mutate: func(r *CrawlRequest) { v := uint32(100); r.MaxPages = &v }},
		{name: "maxPages 0", mutate: func(r *CrawlRequest) { v := uint32(0); r.MaxPages = &v }, wantErr: true},
		{name: "maxPages too large", mutate: func(r *CrawlRequest) { v := uint32(101); r.MaxPages = &v }, wantErr: true},
		{name: "json format", mutate: func(r *CrawlRequest) { r.Formats = []OutputFormat{FormatJson} }, wantErr: true},
		{name: "markdown format ok", mutate: func(r *CrawlRequest) { r.Formats = []OutputFormat{FormatMarkdown} }},
		{name: "invalid renderMode", mutate: func(r *CrawlRequest) { m := RenderMode("rocket"); r.RenderMode = &m }, wantErr: true},
		{name: "waitFor too large", mutate: func(r *CrawlRequest) { v := int64(200000); r.WaitFor = &v }, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := CrawlRequest{URL: "https://example.com"}
			r.Defaults()
			tc.mutate(&r)
			err := r.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestMapRequest_Validate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(r *MapRequest)
		wantErr bool
	}{
		{name: "valid minimal", mutate: func(r *MapRequest) {}},
		{name: "missing url", mutate: func(r *MapRequest) { r.URL = "" }, wantErr: true},
		{name: "bad scheme", mutate: func(r *MapRequest) { r.URL = "javascript:alert(1)" }, wantErr: true},
		{name: "negative maxDepth", mutate: func(r *MapRequest) { v := -1; r.MaxDepth = &v }, wantErr: true},
		{name: "maxDepth too large", mutate: func(r *MapRequest) { v := 101; r.MaxDepth = &v }, wantErr: true},
		{name: "maxDepth 100 ok", mutate: func(r *MapRequest) { v := 100; r.MaxDepth = &v }},
		{name: "zero timeout", mutate: func(r *MapRequest) { v := 0; r.Timeout = &v }, wantErr: true},
		{name: "timeout too large", mutate: func(r *MapRequest) { v := 999999; r.Timeout = &v }, wantErr: true},
		{name: "valid timeout", mutate: func(r *MapRequest) { v := 5000; r.Timeout = &v }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := MapRequest{URL: "https://example.com"}
			r.Defaults()
			tc.mutate(&r)
			err := r.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
