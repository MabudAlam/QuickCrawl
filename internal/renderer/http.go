// Package renderer provides the shared HTTP fetcher used by the
// chromedp-based *core.Scraper. The legacy FallbackRenderer, CDP
// transports, browser pool, and LightPanda support have been removed;
// internal/core now owns the browser pipeline and shares this HTTP
// fetcher for the no-JS path.
package renderer

import (
	"crypto/tls"
	"fmt"
	"io"
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
	transport     *http.Transport
	stealthProfile *utils.HeaderProfile
}

func NewHTTPFetcher(userAgent string, stealthProfile *utils.HeaderProfile) *HTTPFetcher {
	transport := &http.Transport{
		IdleConnTimeout:       60 * time.Second,
		MaxIdleConns:         100,
		MaxIdleConnsPerHost:  10,
		TLSHandshakeTimeout:  HTTPConnectTimeout,
		ResponseHeaderTimeout: HTTPRequestTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			MaxVersion: tls.VersionTLS13,
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   HTTPRequestTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if err := common.ValidateSafeURL(req.URL); err != nil {
				return fmt.Errorf("redirect blocked: %w", err)
			}
			return nil
		},
	}

	return &HTTPFetcher{
		client:        client,
		transport:     transport,
		stealthProfile: stealthProfile,
	}
}

func (f *HTTPFetcher) IsAvailable() bool {
	return true
}

func (f *HTTPFetcher) Name() string {
	return "http"
}

func (f *HTTPFetcher) SupportsJS() bool {
	return false
}

func isRetriableStatus(statusCode int) bool {
	return statusCode >= 502 && statusCode <= 504
}

func isRetriableErrStr(errStr string) bool {
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
		strings.Contains(errStr, "tls handshake") ||
		strings.Contains(errStr, "EOF") ||
		strings.Contains(errStr, "connection reset")
}

func (f *HTTPFetcher) Fetch(rawURL string, headers map[string]string, waitForMs *int64) (*types.FetchResult, *types.QuickCrawlError) {
	start := time.Now()

	deadline := start.Add(HTTPRequestTimeout)

	buildRequest := func() (*http.Request, *types.QuickCrawlError) {
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, types.ErrInvalidURL.New(fmt.Sprintf("Invalid URL: %v", err))
		}

		if f.stealthProfile != nil {
			profileHeaders := f.stealthProfile.ToMap()
			for k, v := range profileHeaders {
				req.Header.Set(k, v)
			}
		}

		for k, v := range headers {
			req.Header.Set(k, v)
		}

		return req, nil
	}

	var lastErr error
	var lastErrStr string

	for attempt := 0; attempt <= httpMaxRetries; attempt++ {
		remaining := deadline.Sub(time.Now())
		if remaining <= 0 {
			return nil, types.ErrHttp.New(fmt.Sprintf("deadline exceeded before HTTP fetch of %s", rawURL))
		}

		if attempt > 0 {
			backoff := httpRetryBackoff
			if backoff > remaining {
				backoff = remaining
			}
			if backoff > 0 {
				time.Sleep(backoff)
			}
		}

		req, reqErr := buildRequest()
		if reqErr != nil {
			return nil, reqErr
		}

		utils.Log.Info("http starting fetch", "url", rawURL, "attempt", attempt)

		resp, err := f.client.Do(req)
		if err != nil {
			lastErr = err
			lastErrStr = err.Error()

			if attempt < httpMaxRetries && isRetriableErrStr(lastErrStr) {
				utils.Log.Info("http transient error", "attempt", attempt, "url", rawURL, "error", err)
				continue
			}

			if strings.Contains(lastErrStr, "connection refused") ||
				strings.Contains(lastErrStr, "no such host") {
				return nil, types.ErrTargetUnreachable.New(fmt.Sprintf("Could not reach %s: %v", rawURL, err))
			}
			return nil, types.ErrHttp.New(err.Error())
		}
		defer resp.Body.Close()

		statusCode := resp.StatusCode

		if attempt < httpMaxRetries && isRetriableStatus(statusCode) {
			lastErr = fmt.Errorf("HTTP %d", statusCode)
			lastErrStr = lastErr.Error()
			utils.Log.Info("http retrying on status", "status", statusCode, "attempt", attempt, "url", rawURL)
			continue
		}

		if cl := resp.Header.Get("Content-Length"); cl != "" {
			if contentLength, err := strconv.ParseInt(cl, 10, 64); err == nil {
				if contentLength > MaxResponseBytes {
					return nil, types.ErrHttp.New(fmt.Sprintf("Response too large: %d bytes (max %d)", contentLength, MaxResponseBytes))
				}
			}
		}

		contentType := resp.Header.Get("Content-Type")
		if idx := strings.Index(contentType, ";"); idx != -1 {
			contentType = strings.TrimSpace(contentType[:idx])
		}
		contentType = strings.ToLower(contentType)

		isPDF := contentType == "application/pdf"

		body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
		if err != nil {
			return nil, types.ErrHttp.New(err.Error())
		}

		if len(body) > MaxResponseBytes {
			return nil, types.ErrHttp.New(fmt.Sprintf("Response too large: %d bytes (max %d)", len(body), MaxResponseBytes))
		}

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

		cfMitigated := resp.Header.Get("cf-mitigated")

		var warning *string
		if statusCode >= 400 {
			warningStr := fmt.Sprintf("Target returned %d %s", statusCode, canonicalStatusText(uint16(statusCode)))
			warning = &warningStr
		} else if cfMitigated == "true" || cfMitigated == "1" {
			warningStr := "cloudflare_mitigated"
			warning = &warningStr
		}

		finalURL := resp.Request.URL.String()
		var finalURLStr *string
		if finalURL != rawURL {
			finalURLStr = &finalURL
		}

		elapsed := time.Since(start)
		utils.Log.Info("http fetch completed", "url", rawURL, "status", statusCode, "duration", elapsed, "size", len(body), "attempt", attempt)

		return &types.FetchResult{
			URL:          rawURL,
			FinalURL:     finalURLStr,
			StatusCode:   uint16(statusCode),
			HTML:         html,
			ContentType:  &contentType,
			RawBytes:     rawBytes,
			RenderedWith: &renderedMethod,
			Warning:      warning,
		}, nil
	}

	if strings.Contains(lastErrStr, "connection refused") ||
		strings.Contains(lastErrStr, "no such host") {
		return nil, types.ErrTargetUnreachable.New(fmt.Sprintf("Could not reach %s after %d attempts: %v", rawURL, httpMaxRetries+1, lastErr))
	}
	return nil, types.ErrHttp.New(fmt.Sprintf("failed after %d attempts: %v", httpMaxRetries+1, lastErr))
}

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
