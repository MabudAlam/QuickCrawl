package core

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/MabudAlam/quickcrawl/internal/utils"
)

type HTTPFetcher struct {
	client         *http.Client
	transport      *http.Transport
	stealthProfile *utils.HeaderProfile
}

func NewHTTPFetcher(userAgent string, stealthProfile *utils.HeaderProfile) *HTTPFetcher {
	transport := &http.Transport{
		IdleConnTimeout:       60 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		TLSHandshakeTimeout:   utils.HTTPConnectTimeout,
		ResponseHeaderTimeout: utils.HTTPRequestTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			MaxVersion: tls.VersionTLS13,
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   utils.HTTPRequestTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if err := utils.ValidateSafeURL(req.URL); err != nil {
				return fmt.Errorf("redirect blocked: %w", err)
			}
			return nil
		},
	}

	return &HTTPFetcher{
		client:         client,
		transport:      transport,
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

// Fetch retrieves rawURL over HTTP, retrying transient failures (recoverable
// network errors and HTTP 5xx) up to utils.HTTPMaxRetries times. Every attempt
// shares one overall time budget (utils.HTTPRequestTimeout). The ctx cancels the
// whole operation — including any in-flight request or retry backoff — so a
// disconnected caller stops the work promptly.
func (f *HTTPFetcher) Fetch(ctx context.Context, rawURL string, headers map[string]string, waitForMs *int64) (*types.FetchResult, *QuickCrawlError) {
	start := time.Now()
	deadline := start.Add(utils.HTTPRequestTimeout)

	// Attempt 0 is the first try; attempts 1..HTTPMaxRetries are the retries.
	for attempt := 0; attempt <= utils.HTTPMaxRetries; attempt++ {
		// 1. Stop as soon as the shared time budget is gone.
		if time.Until(deadline) <= 0 {
			return nil, ErrHttp.New(fmt.Sprintf("deadline exceeded before HTTP fetch of %s", rawURL))
		}

		// 2. Pause between attempts so we don't hammer the server. The sleep
		//    aborts early if the caller's context is canceled.
		if attempt > 0 && !sleepBeforeRetry(ctx, time.Until(deadline)) {
			return nil, ErrHttp.New("HTTP fetch canceled: " + ctx.Err().Error())
		}

		// 3. Fire one GET request.
		req, reqErr := f.newRequest(ctx, rawURL, headers)
		if reqErr != nil {
			return nil, reqErr
		}
		utils.Log.Info("http starting fetch", "url", rawURL, "attempt", attempt)
		resp, err := f.client.Do(req)

		// 4a. Transport-level failure (DNS, TLS, connection, timeout).
		if err != nil {
			// Recoverable network errors are retried while attempts remain.
			if attempt < utils.HTTPMaxRetries && isRetriableErrStr(err.Error()) {
				utils.Log.Info("http transient error", "attempt", attempt, "url", rawURL, "error", err)
				continue
			}
			// Terminal: an unreachable host is a distinct error from other faults.
			if isUnreachableErr(err.Error()) {
				return nil, ErrTargetUnreachable.New(fmt.Sprintf("Could not reach %s: %v", rawURL, err))
			}
			return nil, ErrHttp.New(err.Error())
		}

		// 4b. HTTP 5xx (502-504) usually means the server hiccupped → retry.
		//     Close the body now so we don't hold it open across the retry.
		if attempt < utils.HTTPMaxRetries && isRetriableStatus(resp.StatusCode) {
			_ = resp.Body.Close()
			utils.Log.Info("http retrying on status", "status", resp.StatusCode, "attempt", attempt, "url", rawURL)
			continue
		}
		defer resp.Body.Close()

		// 5. Anything else is a real response: convert body → FetchResult.
		result, qerr := parseResponse(resp, rawURL)
		if qerr != nil {
			return nil, qerr
		}
		utils.Log.Info("http fetch completed", "url", rawURL, "status", result.StatusCode, "duration", time.Since(start), "attempt", attempt)
		return result, nil
	}

	// Unreachable: every iteration above returns, or continues only while
	// attempts remain — so control never falls through the loop.
	return nil, ErrHttp.New(fmt.Sprintf("failed after %d attempts fetching %s", utils.HTTPMaxRetries+1, rawURL))
}

// newRequest builds a GET request for rawURL, layering the stealth-profile
// headers on first and the caller-supplied headers on top, then binding it to
// ctx so the request is aborted if the caller disconnects.
func (f *HTTPFetcher) newRequest(ctx context.Context, rawURL string, headers map[string]string) (*http.Request, *QuickCrawlError) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, ErrInvalidURL.New(fmt.Sprintf("Invalid URL: %v", err))
	}
	if f.stealthProfile != nil {
		for k, v := range f.stealthProfile.ToMap() {
			req.Header.Set(k, v)
		}
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

// sleepBeforeRetry pauses for the configured backoff (capped by the time left in
// the deadline) between attempts. It returns false when the caller's context is
// canceled first, so retries stop immediately on disconnect.
func sleepBeforeRetry(ctx context.Context, remaining time.Duration) bool {
	delay := utils.HTTPRetryBackoff
	if delay > remaining {
		delay = remaining
	}
	if delay <= 0 {
		return true
	}
	select {
	case <-time.After(delay):
		return true
	case <-ctx.Done():
		return false
	}
}

// parseResponse converts a non-retryable HTTP response into a *types.FetchResult.
// It enforces the response-size cap, carries PDF bodies as raw bytes (no HTML),
// and attaches a non-fatal Warning for error statuses / Cloudflare-mitigated pages.
func parseResponse(resp *http.Response, rawURL string) (*types.FetchResult, *QuickCrawlError) {
	// Reject oversized bodies up front when the server sent a Content-Length.
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		if n, err := strconv.ParseInt(cl, 10, 64); err == nil && n > utils.MaxResponseBytes {
			return nil, ErrHttp.New(fmt.Sprintf("Response too large: %d bytes (max %d)", n, utils.MaxResponseBytes))
		}
	}

	// Read the body, capped at MaxResponseBytes+1 so an over-limit body is still
	// detected even when no Content-Length header was sent.
	body, err := io.ReadAll(io.LimitReader(resp.Body, utils.MaxResponseBytes+1))
	if err != nil {
		return nil, ErrHttp.New(err.Error())
	}
	if len(body) > utils.MaxResponseBytes {
		return nil, ErrHttp.New(fmt.Sprintf("Response too large: %d bytes (max %d)", len(body), utils.MaxResponseBytes))
	}

	contentType := cleanContentType(resp.Header.Get("Content-Type"))

	// A PDF has no readable HTML; expose it as raw bytes instead.
	var html string
	var rawBytes []byte
	rendered := "http"
	if contentType == "application/pdf" {
		rawBytes, rendered = body, "pdf"
	} else {
		html = string(body)
	}

	return &types.FetchResult{
		URL:          rawURL,
		FinalURL:     finalURLIfChanged(resp, rawURL),
		StatusCode:   uint16(resp.StatusCode),
		HTML:         html,
		ContentType:  &contentType,
		RawBytes:     rawBytes,
		RenderedWith: &rendered,
		Warning:      responseWarning(resp),
	}, nil
}

// cleanContentType strips parameters (e.g. "; charset=utf-8") and lowercases a
// Content-Type header value.
func cleanContentType(raw string) string {
	if idx := strings.Index(raw, ";"); idx != -1 {
		raw = raw[:idx]
	}
	return strings.ToLower(strings.TrimSpace(raw))
}

// finalURLIfChanged returns the post-redirect URL only when it differs from the
// requested rawURL, so redirects are visible and silent otherwise.
func finalURLIfChanged(resp *http.Response, rawURL string) *string {
	if resp.Request == nil || resp.Request.URL == nil {
		return nil
	}
	final := resp.Request.URL.String()
	if final == rawURL {
		return nil
	}
	return &final
}

// responseWarning records a non-fatal note about the response: an error status
// (>= 400) or a Cloudflare "mitigated" page. Actual failures are returned as
// errors; this is informational only.
func responseWarning(resp *http.Response) *string {
	status := resp.StatusCode
	if status >= 400 {
		msg := fmt.Sprintf("Target returned %d %s", status, canonicalStatusText(uint16(status)))
		return &msg
	}
	if m := resp.Header.Get("cf-mitigated"); m == "true" || m == "1" {
		msg := "cloudflare_mitigated"
		return &msg
	}
	return nil
}

// isUnreachableErr reports whether a transport error means the target host could
// not be reached at all (DNS failure / refused connection), as opposed to an
// arbitrary network fault.
func isUnreachableErr(msg string) bool {
	return strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host")
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
