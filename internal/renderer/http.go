package renderer

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/common"
	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/MabudAlam/quickcrawl/internal/utils"
)

type HTTPFetcher struct {
	client        *http.Client
	stealthProfile *utils.HeaderProfile // nil when stealth is disabled, otherwise provides all headers including User-Agent
}

// NewHTTPFetcher creates a new HTTP fetcher.
// If stealthProfile is nil, uses basic headers from the userAgent string.
// If stealthProfile is provided, uses the full header set from the profile (User-Agent, Accept, Sec-Ch-Ua-*, etc.).
func NewHTTPFetcher(userAgent string, stealthProfile *utils.HeaderProfile) *HTTPFetcher {
	// Configure connection pool for better performance
	transport := &http.Transport{
		IdleConnTimeout:       60 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		TLSHandshakeTimeout:   HTTPConnectTimeout,
		ResponseHeaderTimeout: HTTPRequestTimeout,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Enforce modern TLS versions for security
	transport.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
	}

	// Build HTTP client with custom redirect policy
	client := &http.Client{
		Transport: transport,
		Timeout:   HTTPRequestTimeout,
		// Block redirects to private/localhost URLs for security
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if err := common.ValidateSafeURL(req.URL); err != nil {
				return fmt.Errorf("redirect blocked: %w", err)
			}
			return nil
		},
	}

	return &HTTPFetcher{
		client:        client,
		stealthProfile: stealthProfile,
	}
}

func (f *HTTPFetcher) IsAvailable() bool {
	return true
}

// Fetch performs an HTTP GET request and returns the result.
// It retries once on transient errors (connection/timeout) or gateway errors (502-504).
func (f *HTTPFetcher) Fetch(rawURL string, headers map[string]string, waitForMs *int64) (*types.FetchResult, *types.QuickCrawlError) {
	start := time.Now() // Record start time to measure total fetch duration

	// buildRequest creates a new http.Request for each fetch attempt.
	// Headers are rebuilt fresh each time to ensure they're not mutated by the transport.
	buildRequest := func() (*http.Request, *types.QuickCrawlError) {
		// Create new HTTP GET request with no body
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			// Invalid URL format (malformed URL) - return error immediately, no retry
			return nil, types.ErrInvalidURL.New(fmt.Sprintf("Invalid URL: %v", err))
		}

		// If stealth mode is enabled, use the full header set from stealthProfile.
		// This includes User-Agent, Accept, Sec-Ch-Ua-*, Sec-Fetch-* headers for realistic browser traffic.
		// If stealth is disabled (stealthProfile is nil), start with empty headers.
		if f.stealthProfile != nil {
			profileHeaders := f.stealthProfile.ToMap()
			for k, v := range profileHeaders {
				req.Header.Set(k, v)
			}
		}

		// Apply caller-provided headers on top (e.g., custom Accept, Authorization, custom User-Agent override)
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		return req, nil
	}

	// isRetriableError checks if an error is transient and worth retrying.
	// Certificate errors (expired, invalid) are NOT retried as they are permanent failures.
	isRetriableError := func(err error) bool {
		if err == nil {
			return false
		}
		errStr := err.Error()
		// Certificate issues indicate server misconfiguration, not transient network issues
		if strings.Contains(errStr, "certificate has expired") ||
			strings.Contains(errStr, "x509") ||
			strings.Contains(errStr, "certificate is valid for") {
			return false
		}
		// Transient network errors that may resolve on retry
		return strings.Contains(errStr, "connection refused") ||
			strings.Contains(errStr, "timeout") ||
			strings.Contains(errStr, "deadline exceeded") ||
			strings.Contains(errStr, "no such host") ||
			strings.Contains(errStr, "temporary failure") ||
			strings.Contains(errStr, "i/o timeout") ||
			strings.Contains(errStr, "tls handshake")
	}

	// isRetriableStatus returns true for transient gateway errors (502-504).
	// These errors from reverse proxies often resolve on retry.
	isRetriableStatus := func(statusCode int) bool {
		return statusCode >= 502 && statusCode <= 504
	}

	var lastErr error // Track the most recent error for final error message
	// httpMaxRetries=1 allows 2 total attempts
	for attempt := 0; attempt <= httpMaxRetries; attempt++ {
		if attempt > 0 {
			// Before retrying, apply backoff delay to avoid hammering a struggling server
			backoff := httpRetryBackoff
			log.Printf("[http] retrying (attempt %d): url=%s backoff=%v", attempt, rawURL, backoff)
			time.Sleep(backoff)
		}

		// Build fresh request with headers (each attempt gets new headers in case they were mutated)
		req, reqErr := buildRequest()
		if reqErr != nil {
			return nil, reqErr // Invalid URL error - no point retrying
		}

		log.Printf("[http] starting fetch: url=%s attempt=%d", rawURL, attempt)

		// Execute the HTTP request using the pre-configured client
		resp, err := f.client.Do(req)
		if err != nil {
			lastErr = err
			// For transient errors, retry if we haven't exhausted retries
			if attempt < httpMaxRetries && isRetriableError(err) {
				log.Printf("[http] transient error (attempt %d): url=%s error=%v", attempt, rawURL, err)
				continue // Jump to next iteration (after backoff)
			}
			// For DNS/connection errors (host unreachable), return specific error type
			if strings.Contains(err.Error(), "connection refused") ||
				strings.Contains(err.Error(), "no such host") {
				return nil, types.ErrTargetUnreachable.New(fmt.Sprintf("Could not reach %s: %v", rawURL, err))
			}
			// All other errors (certificate, TLS, etc.) - return generic HTTP error
			return nil, types.ErrHttp.New(err.Error())
		}
		defer resp.Body.Close() // Ensure response body is closed, releasing connection back to pool

		statusCode := resp.StatusCode // Extract HTTP status code (e.g., 200, 404, 500)

		// Retry on 502-504 gateway errors with one retry
		if attempt < httpMaxRetries && isRetriableStatus(statusCode) {
			lastErr = fmt.Errorf("HTTP %d", statusCode)
			log.Printf("[http] retrying on status %d (attempt %d): url=%s", statusCode, attempt, rawURL)
			continue // Retry the request
		}

		// Check Content-Length header to reject oversized responses before reading body.
		// This prevents downloading huge files that would exceed MaxResponseBytes.
		if cl := resp.Header.Get("Content-Length"); cl != "" {
			if contentLength, err := strconv.ParseInt(cl, 10, 64); err == nil {
				if contentLength > MaxResponseBytes {
					return nil, types.ErrHttp.New(fmt.Sprintf("Response too large: %d bytes (max %d)", contentLength, MaxResponseBytes))
				}
			}
		}

		// Extract and normalize Content-Type header.
		// Remove charset parameter (e.g., "text/html; charset=utf-8" -> "text/html").
		contentType := resp.Header.Get("Content-Type")
		if idx := strings.Index(contentType, ";"); idx != -1 {
			contentType = strings.TrimSpace(contentType[:idx])
		}
		contentType = strings.ToLower(contentType) // Normalize to lowercase for comparison

		// Check if response is a PDF (binary content type)
		isPDF := contentType == "application/pdf"

		// Read response body with size limit using LimitReader to prevent unbounded memory allocation.
		// Read one byte more than MaxResponseBytes so we can detect overflow.
		body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
		if err != nil {
			return nil, types.ErrHttp.New(err.Error())
		}

		// Verify actual body size after reading (Content-Length might not have been set)
		if len(body) > MaxResponseBytes {
			return nil, types.ErrHttp.New(fmt.Sprintf("Response too large: %d bytes (max %d)", len(body), MaxResponseBytes))
		}

		// Prepare result based on content type
		var html string   // For HTML/text content, store as string
		var rawBytes []byte // For binary content (PDFs), store raw bytes
		renderedMethod := "http" // Indicates how content was rendered (http, browser, pdf)

		if isPDF {
			// PDF content: store raw bytes, no HTML
			rawBytes = body
			html = ""
			renderedMethod = "pdf"
		} else {
			// HTML/text content: convert body bytes to string
			html = string(body)
		}

		// Add warning for HTTP error status codes (4xx client errors, 5xx server errors).
		// This doesn't fail the request, just warns the caller.
		var warning *string
		if statusCode >= 400 {
			warningStr := fmt.Sprintf("Target returned %d %s", statusCode, canonicalStatusText(uint16(statusCode)))
			warning = &warningStr
		}

		elapsed := time.Since(start)
		log.Printf("[http] fetch completed: url=%s status=%d duration=%v size=%d attempt=%d", rawURL, statusCode, elapsed, len(body), attempt)

		// Return successful result with all fetched data and metadata
		return &types.FetchResult{
			URL:          rawURL,
			StatusCode:   uint16(statusCode),
			HTML:         html,
			ContentType:  &contentType,
			RawBytes:     rawBytes,
			RenderedWith: &renderedMethod,
			TimeTaken:    uint64(time.Since(start).Milliseconds()),
			Warning:      warning,
		}, nil
	}

	// All retries exhausted - this point is only reached if all attempts failed
	if strings.Contains(lastErr.Error(), "connection refused") ||
		strings.Contains(lastErr.Error(), "no such host") {
		// Distinguish unreachable hosts from other errors
		return nil, types.ErrTargetUnreachable.New(fmt.Sprintf("Could not reach %s after %d attempts: %v", rawURL, httpMaxRetries+1, lastErr))
	}
	// Generic HTTP error for other failure types
	return nil, types.ErrHttp.New(fmt.Sprintf("failed after %d attempts: %v", httpMaxRetries+1, lastErr))
}

// canonicalStatusText converts HTTP status codes to human-readable text.
func canonicalStatusText(code uint16) string {
	switch code {
	case 400:
		return "Bad Request"
	case 401:
		return "Unauthorized"
	case 403:
		return "Forbidden"
	case 404:
		return "Not Found"
	case 405:
		return "Method Not Allowed"
	case 408:
		return "Request Timeout"
	case 410:
		return "Gone"
	case 429:
		return "Too Many Requests"
	case 451:
		return "Unavailable For Legal Reasons"
	case 500:
		return "Internal Server Error"
	case 502:
		return "Bad Gateway"
	case 503:
		return "Service Unavailable"
	case 504:
		return "Gateway Timeout"
	default:
		return "Error"
	}
}