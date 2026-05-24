package renderer

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/common"
	"github.com/MabudAlam/quickcrawl/internal/types"
)

// HTTPFetcher retrieves pages using plain HTTP requests.
// It cannot execute JavaScript, so SPAs will show as minimal content.
type HTTPFetcher struct {
	client               *http.Client
	injectStealthHeaders bool
	userAgent            string
}

// NewHTTPFetcher creates a new HTTP fetcher with the given settings.
func NewHTTPFetcher(userAgent string, injectStealthHeaders bool) *HTTPFetcher {
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
		// Block redirects to private/localhost URLs
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if err := common.ValidateSafeURL(req.URL); err != nil {
				return fmt.Errorf("redirect blocked: %w", err)
			}
			return nil
		},
	}

	return &HTTPFetcher{
		client:               client,
		injectStealthHeaders: injectStealthHeaders,
		userAgent:            userAgent,
	}
}

// Name returns the fetcher type name.
func (f *HTTPFetcher) Name() string {
	return "http"
}

// SupportsJS returns false since HTTP fetcher cannot run JavaScript.
func (f *HTTPFetcher) SupportsJS() bool {
	return false
}

// IsAvailable always returns true for HTTP fetcher.
func (f *HTTPFetcher) IsAvailable() bool {
	return true
}

// Fetch performs an HTTP GET request and returns the result.
// It retries once on transient errors (connection/timeout) or gateway errors (502-504).
func (f *HTTPFetcher) Fetch(rawURL string, headers map[string]string, waitForMs *int64) (*types.FetchResult, *types.QuickCrawlError) {
	start := time.Now()

	// Build request once; we rebuild it each attempt so headers are fresh.
	buildRequest := func() (*http.Request, *types.QuickCrawlError) {
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, types.ErrInvalidURL.New(fmt.Sprintf("Invalid URL: %v", err))
		}
		req.Header.Set("User-Agent", f.userAgent)
		if f.injectStealthHeaders {
			for k, v := range stealthHeaders {
				req.Header.Set(k, v)
			}
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		return req, nil
	}

	// isRetriableError returns true for transient errors worth one retry.
// Does NOT retry certificate errors (expired, invalid) as these are permanent.
	isRetriableError := func(err error) bool {
		if err == nil {
			return false
		}
		errStr := err.Error()
		// Skip retryable check for certificate errors - these are permanent failures
		if strings.Contains(errStr, "certificate has expired") ||
			strings.Contains(errStr, "x509") ||
			strings.Contains(errStr, "certificate is valid for") {
			return false
		}
		return strings.Contains(errStr, "connection refused") ||
			strings.Contains(errStr, "timeout") ||
			strings.Contains(errStr, "deadline exceeded") ||
			strings.Contains(errStr, "no such host") ||
			strings.Contains(errStr, "temporary failure") ||
			strings.Contains(errStr, "i/o timeout") ||
			strings.Contains(errStr, "tls handshake")
	}

	// isRetriableStatus returns true for transient gateway errors.
	isRetriableStatus := func(statusCode int) bool {
		return statusCode >= 502 && statusCode <= 504
	}

	var lastErr error
	for attempt := 0; attempt <= httpMaxRetries; attempt++ {
		if attempt > 0 {
			backoff := httpRetryBackoff
			log.Printf("[http] retrying (attempt %d): url=%s backoff=%v", attempt, rawURL, backoff)
			time.Sleep(backoff)
		}

		req, reqErr := buildRequest()
		if reqErr != nil {
			return nil, reqErr
		}

		log.Printf("[http] starting fetch: url=%s attempt=%d", rawURL, attempt)

		resp, err := f.client.Do(req)
		if err != nil {
			lastErr = err
			if attempt < httpMaxRetries && isRetriableError(err) {
				log.Printf("[http] transient error (attempt %d): url=%s error=%v", attempt, rawURL, err)
				continue
			}
			if strings.Contains(err.Error(), "connection refused") ||
				strings.Contains(err.Error(), "no such host") {
				return nil, types.ErrTargetUnreachable.New(fmt.Sprintf("Could not reach %s: %v", rawURL, err))
			}
			return nil, types.ErrHttp.New(err.Error())
		}
		defer resp.Body.Close()

		statusCode := resp.StatusCode

		// Retry on 502-504 with one retry
		if attempt < httpMaxRetries && isRetriableStatus(statusCode) {
			lastErr = fmt.Errorf("HTTP %d", statusCode)
			log.Printf("[http] retrying on status %d (attempt %d): url=%s", statusCode, attempt, rawURL)
			continue
		}

		// Check Content-Length header for size limits
		if cl := resp.Header.Get("Content-Length"); cl != "" {
			if contentLength, err := strconv.ParseInt(cl, 10, 64); err == nil {
				if contentLength > MaxResponseBytes {
					return nil, types.ErrHttp.New(fmt.Sprintf("Response too large: %d bytes (max %d)", contentLength, MaxResponseBytes))
				}
			}
		}

		// Extract and normalize Content-Type
		contentType := resp.Header.Get("Content-Type")
		if idx := strings.Index(contentType, ";"); idx != -1 {
			contentType = strings.TrimSpace(contentType[:idx])
		}
		contentType = strings.ToLower(contentType)

		// Check if response is a PDF
		isPDF := contentType == "application/pdf"

		// Read response body with size limit
		body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
		if err != nil {
			return nil, types.ErrHttp.New(err.Error())
		}

		// Verify body size after reading (Content-Length might not be set)
		if len(body) > MaxResponseBytes {
			return nil, types.ErrHttp.New(fmt.Sprintf("Response too large: %d bytes (max %d)", len(body), MaxResponseBytes))
		}

		// Prepare result based on content type
		var html string
		var rawBytes []byte
		renderedMethod := "http"

		if isPDF {
			rawBytes = body
			html = ""
			renderedMethod = "pdf"
		} else {
			html = string(body)
		}

		// Add warning for error status codes
		var warning *string
		if statusCode >= 400 {
			warningStr := fmt.Sprintf("Target returned %d %s", statusCode, canonicalStatusText(uint16(statusCode)))
			warning = &warningStr
		}

		elapsed := time.Since(start)
		log.Printf("[http] fetch completed: url=%s status=%d duration=%v size=%d attempt=%d", rawURL, statusCode, elapsed, len(body), attempt)

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

	// All retries exhausted
	if strings.Contains(lastErr.Error(), "connection refused") ||
		strings.Contains(lastErr.Error(), "no such host") {
		return nil, types.ErrTargetUnreachable.New(fmt.Sprintf("Could not reach %s after %d attempts: %v", rawURL, httpMaxRetries+1, lastErr))
	}
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

// selectUserAgent selects a User-Agent string based on stealth settings.
// If stealth is enabled, it randomly picks from the configured pool to mimic real browsers.
func selectUserAgent(defaultUA string, stealth *types.StealthConfig) string {
	if stealth != nil && stealth.Enabled {
		if len(stealth.UserAgents) > 0 {
			return stealth.UserAgents[rand.Intn(len(stealth.UserAgents))]
		}
		return common.BuiltinUAPool[rand.Intn(len(common.BuiltinUAPool))]
	}
	return defaultUA
}
