package renderer

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPFetcher_IsAvailable(t *testing.T) {
	fetcher := NewHTTPFetcher("", nil)
	if !fetcher.IsAvailable() {
		t.Errorf("expected HTTPFetcher to be available")
	}
}

func TestHTTPFetcher_Name(t *testing.T) {
	fetcher := NewHTTPFetcher("", nil)
	if fetcher.Name() != "http" {
		t.Errorf("expected name 'http', got %q", fetcher.Name())
	}
}

func TestHTTPFetcher_SupportsJS(t *testing.T) {
	fetcher := NewHTTPFetcher("", nil)
	if fetcher.SupportsJS() {
		t.Errorf("expected HTTPFetcher to not support JS")
	}
}

func TestIsRetriableStatus(t *testing.T) {
	tests := []struct {
		statusCode int
		expected   bool
	}{
		{200, false},
		{400, false},
		{404, false},
		{500, false},
		{501, false},
		{502, true},
		{503, true},
		{504, true},
		{505, false},
	}

	for _, tc := range tests {
		got := isRetriableStatus(tc.statusCode)
		if got != tc.expected {
			t.Errorf("isRetriableStatus(%d) = %v, want %v", tc.statusCode, got, tc.expected)
		}
	}
}

func TestIsRetriableErrStr(t *testing.T) {
	retriableErrors := []string{
		"connection refused",
		"timeout",
		"deadline exceeded",
		"no such host",
		"temporary failure",
		"i/o timeout",
		"tls handshake",
		"EOF",
		"connection reset",
	}

	nonRetriableErrors := []string{
		"certificate has expired",
		"x509",
		"certificate is valid for",
	}

	for _, errStr := range retriableErrors {
		if !isRetriableErrStr(errStr) {
			t.Errorf("expected %q to be retriable", errStr)
		}
	}

	for _, errStr := range nonRetriableErrors {
		if isRetriableErrStr(errStr) {
			t.Errorf("expected %q to NOT be retriable", errStr)
		}
	}
}

func TestHTTPFetcher_Fetch_BasicHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(200)
		w.Write([]byte("<html><body><h1>Test</h1></body></html>"))
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher("", nil)
	result, err := fetcher.Fetch(server.URL, nil, nil)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatalf("expected result, got nil")
	}
	if result.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", result.StatusCode)
	}
	if !strings.Contains(result.HTML, "<h1>Test</h1>") {
		t.Errorf("expected HTML to contain <h1>Test</h1>, got %q", result.HTML)
	}
}

func TestHTTPFetcher_Fetch_InvalidURL(t *testing.T) {
	fetcher := NewHTTPFetcher("", nil)
	result, err := fetcher.Fetch("http://invalid-url-that-does-not-exist", nil, nil)

	if err == nil {
		t.Errorf("expected error for invalid URL")
	}
	if result != nil {
		t.Errorf("expected nil result for invalid URL")
	}
}

func TestHTTPFetcher_Fetch_SetsCustomHeaders(t *testing.T) {
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(200)
		w.Write([]byte("<html><body>OK</body></html>"))
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher("", nil)
	customHeaders := map[string]string{"X-Custom-Header": "test-value"}
	_, _ = fetcher.Fetch(server.URL, customHeaders, nil)

	if receivedHeaders.Get("X-Custom-Header") != "test-value" {
		t.Errorf("expected X-Custom-Header 'test-value', got %q", receivedHeaders.Get("X-Custom-Header"))
	}
}

func TestCanonicalStatusText(t *testing.T) {
	tests := []struct {
		code     uint16
		expected string
	}{
		{400, "Bad Request"},
		{401, "Unauthorized"},
		{403, "Forbidden"},
		{404, "Not Found"},
		{408, "Request Timeout"},
		{429, "Too Many Requests"},
		{500, "Internal Server Error"},
		{502, "Bad Gateway"},
		{503, "Service Unavailable"},
		{504, "Gateway Timeout"},
		{999, "Error"},
	}

	for _, tc := range tests {
		got := canonicalStatusText(tc.code)
		if got != tc.expected {
			t.Errorf("canonicalStatusText(%d) = %q, want %q", tc.code, got, tc.expected)
		}
	}
}
