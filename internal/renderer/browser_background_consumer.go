package renderer

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/types"
)

// Network capture limits for XHR/Fetch response bodies.
const (
	netCaptureMaxBodies      = 30                     // Maximum number of captured responses to store.
	netCaptureMaxTotalBytes  = 2_000_000              // Maximum total bytes across all captured responses (2 MB).
	netCaptureMinBodySize    = 512                    // Minimum body size in bytes to qualify for capture.
	netCaptureGetBodyTimeout = 800 * time.Millisecond // Timeout for fetching response body via CDP.
	requestPausedTimeout     = 2 * time.Second        // Timeout for Fetch.requestPaused / Fetch.authRequired CDP commands.
)

// defaultBlockedResourceTypes are CDP network resource types that are blocked
// before any HTTP request is made. Blocking heavy resource types reduces
// bandwidth and speeds up page load during scraping.
var defaultBlockedResourceTypes = []string{
	"Image",     // Reduces memory pressure and bandwidth.
	"Media",     // Audio/video files are rarely needed for content extraction.
	"Font",      // WOFF/TTF files can be large; page remains readable without them.
	"Manifest",  // App manifest files are irrelevant for scraping.
	"WebSocket", // WebSocket connections are typically用于实时数据而非静态内容.
}

// defaultBlockedHosts is a list of known third-party host substrings that are
// blocked during scraping. These hosts serve analytics, tracking, ads, chat
// widgets, consent banners, and other non-content scripts. Blocking them:
//
//   - Reduces page load time and network overhead.
//   - Prevents analytics/ads requests from polluting CapturedResponses.
//   - Avoids CMP banners covering page content.
//   - Improves SPA readiness detection accuracy.
var defaultBlockedHosts = []string{

	// ─── Analytics ───────────────────────────────────────────────────────────────
	"amplitude.com",        // Behavioral analytics platform.
	"mixpanel.com",         // Product analytics and engagement.
	"google-analytics.com", // Google Analytics (GA4 / Universal Analytics).
	"googletagmanager.com", // Google Tag Manager container scripts.
	"clarity.ms",           // Microsoft Clarity heatmaps and session recordings.

	// ─── Advertising ─────────────────────────────────────────────────────────────
	"doubleclick.net",       // Google Display Ads.
	"googleadservices.com",  // Google Ads conversion scripts.
	"googlesyndication.com", // Google AdSense.
	"adsystem.com",          // Generic ad system scripts.
	"adservice.google.com",  // Google Ads resource calls.
	"taboola.com",           // Taboola recommendation widget.
	"outbrain.com",          // Outbrain recommendation engine.
	"criteo.com",            // Criteo retargeting.
	"criteo.net",            // Criteo network resources.
	"scorecardresearch.com", // Scorecard Research (comScore).
	"quantserve.com",        // Quantcast measurement.

	// ─── Session Recording / Heavy Analytics ─────────────────────────────────────
	"fullstory.com",     // FullStory session replay and heatmaps.
	"heap.io",           // Heap auto-capture analytics.
	"heapanalytics.com", // Heap analytics domain variant.
	"logrocket.com",     // LogRocket session replay.
	"mouseflow.com",     // Mouseflow heatmaps and funnels.
	"smartlook.com",     // SmartLook session recording.
	"luckyorange.com",   // Lucky Orange chat and replay.

	// ─── Consent / GDPR / Privacy Banners ────────────────────────────────────────
	"onetrust.com",     // OneTrust cookie consent platform.
	"cookielaw.org",    // Cookie Law compliance notice scripts.
	"cookiebot.com",    // CookieBot GDPR consent manager.
	"trustarc.com",     // TrustArc privacy management.
	"usercentrics.com", // Usercentrics Consent Management Platform.
	"didomi.io",        // Didomi privacy and consent platform.
	"quantcastmgr.com", // Quantcast Choice (IAB CMP).
	"iubenda.com",      // Iubenda cookie consent and privacy.
	"osano.com",        // Osano cookie consent manager.

	// ─── Tag Managers / Data Pipelines ───────────────────────────────────────────
	"segment.io",            // Segment customer data platform.
	"segment.com",           // Segment primary domain.
	"mparticle.com",         // mParticle customer data platform.
	"rudderstack.com",       // RudderStack data pipeline.
	"snowplowanalytics.com", // Snowplow Analytics.
	"snowplow.io",           // Snowplow tracking endpoint.
	"tealium.com",           // Tealium tag management.
	"ensighten.com",         // Ensighten tag governance.
	"adobe-analytics.com",   // Adobe Analytics (Omniture).
	"launch.adobe.com",      // Adobe Launch (Tag Management).

	// ─── Error / Performance Monitoring ──────────────────────────────────────────
	"newrelic.com",    // New Relic APM and browser monitoring.
	"nr-data.net",     // New Relic data collection endpoint.
	"bugsnag.com",     // Bugsnag error monitoring.
	"rollbar.com",     // Rollbar error tracking.
	"airbrake.io",     // Airbrake application error monitoring.
	"sentry.io",       // Sentry error and performance monitoring.
	"datadoghq.com",   // Datadog US endpoint.
	"datadoghq.eu",    // Datadog EU endpoint.
	"dynatrace.com",   // Dynatrace application performance monitoring.
	"appdynamics.com", // AppDynamics APM.
	"instana.io",      // Instana automated APM.

	// ─── Chat / Support Widgets ───────────────────────────────────────────────────
	"intercom.io",      // Intercom customer messaging.
	"intercomcdn.com",  // Intercom assets CDN.
	"zendesk.com",      // Zendesk support widget.
	"zendesk.is",       // Zendesk embedded resources.
	"drift.com",        // Drift conversational marketing/chat.
	"hubspot.com",      // HubSpot chat and CRM embed.
	"hs-analytics.net", // HubSpot analytics scripts.
	"hs-scripts.com",   // HubSpot tracking pixels.
	"freshdesk.com",    // Freshdesk support widget.
	"livechat.com",     // LiveChat widget.
	"olark.com",        // Olark live chat.
	"tawk.to",          // Tawk.to free chat widget.
	"crisp.chat",       // Crisp live chat and messaging.

	// ─── Social Sharing Widgets ─────────────────────────────────────────────────
	"addthis.com",   // AddThis social sharing buttons.
	"sharethis.com", // ShareThis sharing platform.
	"addtoany.com",  // AddToAny share buttons.
	"sumome.com",    // SumoMe social sharing and analytics.

	// ─── A/B Testing / Personalization ──────────────────────────────────────────
	"optimizely.com",   // Optimizely experimentation platform.
	"vwo.com",          // VWO (Visual Website Optimizer) A/B testing.
	"dynamicyield.com", // Dynamic Yield personalization engine.
	"kameleoon.com",    // Kameleoon feature flag and experimentation.
	"abtasty.com",      // AB Tasty experimentation and personalisation.
	"chartbeat.com",    // Chartbeat real-time analytics and content performance.

	// ─── Retargeting / Tracking Pixels ──────────────────────────────────────────
	"connect.facebook.net",  // Facebook Pixel and SDK.
	"pixel.facebook.com",    // Facebook conversion pixel.
	"ads.linkedin.com",      // LinkedIn Ads tracking pixel.
	"analytics.twitter.com", // Twitter analytics and tracking.
	"t.co",                  // Twitter URL shortener (tracks clicks).
	"analytics.tiktok.com",  // TikTok analytics and pixel.
	"s.pinimg.com",          // Pinterest tracking pixel assets.
	"sc-static.net",         // Snapchat Pixel resources.
	"reddit.com/pixel",      // Reddit conversion tracking pixel.

	// ─── PostHog ────────────────────────────────────────────────────────────────
	"posthog.com",     // PostHog product analytics (main domain).
	"app.posthog.com", // PostHog dashboard application.
	"eu.posthog.com",  // PostHog EU-hosted instance.
	"us.posthog.com",  // PostHog US-hosted instance.
	"cdn.posthog.com", // PostHog JS library CDN.

	// ─── Surveys / Feedback Forms ───────────────────────────────────────────────
	"typeform.com",  // Typeform embedded forms and surveys.
	"qualtrics.com", // Qualtrics experience management surveys.
	"usabilla.com",  // Usabilla feedback and survey tool.
	"uservoice.com", // UserVoice feedback widget.
	"qualaroo.com",  // Qualaroo in-app surveys.
}

// blockReason describes why a request was blocked — either by resource type
// (e.g., Image, Media) or by host matching the blocklist.
type blockReason string

// Block reason constants used for logging and debugging purposes.
const (
	blockReasonResourceType blockReason = "resource_type" // Blocked because the CDP resource type is in defaultBlockedResourceTypes.
	blockReasonHost         blockReason = "host"          // Blocked because the request host matches a substring in defaultBlockedHosts.
)

// requestBlocklist holds compiled lookup structures for fast blocklist matching.
// It is safe to use concurrently from multiple goroutines.
type requestBlocklist struct {
	resourceTypes    map[string]struct{} // Set of blocked CDP resource types (e.g., "Image", "Media").
	hostSubstrings   []string            // Lowercase substrings to match against request hostname.
	blockStylesheets bool                // Whether to also block Stylesheet requests (currently disabled by default).
}

// newDefaultRequestBlocklist builds a requestBlocklist from the global
// defaultBlockedResourceTypes and defaultBlockedHosts variables.
// The resource types are stored in a map for O(1) lookup; host substrings
// are lowercased once at construction time.
func newDefaultRequestBlocklist() requestBlocklist {
	// Build O(1) resource-type lookup map from the global list.
	resourceTypes := make(map[string]struct{}, len(defaultBlockedResourceTypes))
	for _, resourceType := range defaultBlockedResourceTypes {
		resourceTypes[resourceType] = struct{}{}
	}
	// Pre-lowercase all host substrings so matching is case-insensitive.
	hostSubstrings := make([]string, 0, len(defaultBlockedHosts))
	for _, host := range defaultBlockedHosts {
		hostSubstrings = append(hostSubstrings, strings.ToLower(host))
	}
	return requestBlocklist{
		resourceTypes:    resourceTypes,
		hostSubstrings:   hostSubstrings,
		blockStylesheets: false, // Stylesheets are allowed by default to preserve page layout.
	}
}

// shouldBlock checks whether a given CDP resource type and URL should be
// blocked based on the compiled blocklist. It first checks resource type,
// then checks hostname substring match if no resource-type match is found.
//
// Returns the reason (resourceType or host) and true if the request should be
// blocked; otherwise returns empty string and false.
func (b requestBlocklist) shouldBlock(resourceType, requestURL string) (blockReason, bool) {
	// Fast O(1) check against the resource-type set.
	if _, ok := b.resourceTypes[resourceType]; ok {
		return blockReasonResourceType, true
	}
	// Stylesheets can be optionally blocked (controlled by blockStylesheets flag).
	if b.blockStylesheets && resourceType == "Stylesheet" {
		return blockReasonResourceType, true
	}
	// No host substrings configured — allow everything else.
	if len(b.hostSubstrings) == 0 {
		return "", false
	}
	// Parse the request URL to extract the hostname for matching.
	parsed, err := url.Parse(requestURL)
	if err != nil {
		return "", false
	}
	// Normalize hostname to lowercase for case-insensitive comparison.
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return "", false
	}
	// Linear scan over host substrings. Uses strings.Contains so a needle
	// like "doubleclick.net" matches "ads.doubleclick.net".
	for _, needle := range b.hostSubstrings {
		if strings.Contains(host, needle) {
			return blockReasonHost, true
		}
	}
	return "", false
}

// capturedNetworkResponse holds a captured XHR or Fetch response body
// along with metadata. Used internally by runNetworkCapturePump to
// accumulate responses before converting to the public types.CapturedNetworkResponse.
type capturedNetworkResponse struct {
	URL           string  // Full URL of the captured request.
	RequestID     string  // CDP request ID used to fetch the response body.
	Status        uint16  // HTTP status code (e.g., 200, 201).
	MimeType      *string // Content-Type MIME type of the response (pointer to distinguish "unset" from empty).
	BodySizeBytes int     // Size of the response body in bytes.
	Body          *string // Actual response body text (nil if base64-encoded or too large).
}

// toCapturedNetworkResponses converts a slice of internal capturedNetworkResponse
// structs into the public types.CapturedNetworkResponse type expected by
// the rest of the codebase.
func toCapturedNetworkResponses(in []capturedNetworkResponse) []types.CapturedNetworkResponse {
	// Pre-allocate output slice with exact capacity to avoid reallocation.
	out := make([]types.CapturedNetworkResponse, 0, len(in))
	for _, item := range in {
		out = append(out, types.CapturedNetworkResponse{
			URL:           item.URL,
			RequestID:     item.RequestID,
			Status:        item.Status,
			MimeType:      item.MimeType,
			BodySizeBytes: item.BodySizeBytes,
			Body:          item.Body,
		})
	}
	return out
}

// isCapturableMIME returns true if the given MIME type is one that
// we want to capture (JSON or plain text). Responses with other MIME
// types (HTML, XML, binary, etc.) are skipped.
func isCapturableMIME(mime string) bool {
	// Normalize: lowercase, trim whitespace, strip charset suffix.
	m := strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	switch m {
	// JSON variants commonly used by SPAs and APIs.
	case "application/json", "application/ld+json", "application/vnd.api+json", "text/json":
		return true
	// Plain text API responses (less common but sometimes used).
	case "text/plain":
		return true
	default:
		return false
	}
}

// buildAuthResponse constructs a CDP Fetch.authChallengeResponse payload.
// If credentials are provided, it returns ProvideCredentials with the username
// and password. Otherwise it returns CancelAuth to abort the request.
func buildAuthResponse(requestID string, creds *[2]string) map[string]any {
	if creds != nil {
		// Provide the stored credentials to the server.
		return map[string]any{
			"requestId": requestID,
			"authChallengeResponse": map[string]any{
				"response": "ProvideCredentials", // Tells CDP to send auth with provided username/password.
				"username": creds[0],
				"password": creds[1],
			},
		}
	}
	// Cancel auth — the request will fail with an auth error.
	return map[string]any{
		"requestId": requestID,
		"authChallengeResponse": map[string]any{
			"response": "CancelAuth", // Abort the request without sending credentials.
		},
	}
}

// runFetchInterceptionPump is a long-running goroutine that processes
// CDP Fetch.requestPaused events. It is started when Fetch.enable is called
// with patterns configured. For each paused request it:
//
//  1. Extracts the requestId, resourceType, and request URL from the event.
//  2. Checks the blocklist — if matched, fails the request with BlockedByClient.
//  3. If not blocked, continues the request unchanged.
//
// The pump runs until done is closed. It does not return a value; results
// are observed through blocked network requests and log output.
func runFetchInterceptionPump(
	conn *cdpConnection, // Active CDP connection for sending commands.
	events <-chan cdpEvent, // Channel receiving all CDP events for this session.
	sessionID string, // CDP session ID to filter events by this session only.
	blocklist requestBlocklist, // Compiled blocklist for resource-type and host matching.
	done <-chan struct{}, // Close signal to gracefully shut down the pump.
) {
	for {
		select {
		case <-done:
			// Shutdown signal received — exit the pump goroutine.
			return
		case ev, ok := <-events:
			// Channel closed or event available.
			if !ok {
				// CDP connection closed the event channel — exit.
				return
			}
			// Filter to only Fetch.requestPaused events for our session.
			if ev.Method != "Fetch.requestPaused" || ev.SessionID != sessionID {
				continue
			}
			// Extract requestId — required for every Fetch command.
			requestID := extractEventString(ev.Params, "requestId")
			if requestID == "" {
				continue
			}
			// Determine the CDP resource type (Script, Stylesheet, Image, XHR, etc.).
			resourceType := extractEventString(ev.Params, "resourceType")
			// Extract the full request URL to check against the host blocklist.
			requestURL := extractNestedEventString(ev.Params, "request", "url")
			// Check blocklist: resource type first, then hostname substring match.
			if reason, ok := blocklist.shouldBlock(resourceType, requestURL); ok {
				// Block the request — tell the browser to abort with a client-side block reason.
				_, _ = conn.SendRecv(
					"Fetch.failRequest",
					map[string]any{
						"requestId":   requestID,
						"errorReason": "BlockedByClient", // Browser aborts with net::ERR_BLOCKED_BY_CLIENT.
					},
					sessionID,
					requestPausedTimeout,
				)
				// Log reason (currently discarded with _ = reason; could be surfaced in metrics).
				_ = reason
				continue
			}
			// Not blocked — pass the request through to the server unchanged.
			_, _ = conn.SendRecv(
				"Fetch.continueRequest",
				map[string]any{"requestId": requestID},
				sessionID,
				requestPausedTimeout,
			)
		}
	}
}

// runFetchAuthPump handles CDP Fetch.authRequired events for HTTP Basic/Digest
// authentication challenges. For each authRequired event it either provides
// stored credentials (if available) or cancels the auth challenge, causing the
// request to fail with an auth error.
//
// This pump is currently compiled in but disabled (authEnabled = false in
// browser_fetcher.go). It is useful for sites behind HTTP authentication.
func runFetchAuthPump(
	conn *cdpConnection, // Active CDP connection for sending commands.
	events <-chan cdpEvent, // Channel receiving all CDP events for this session.
	sessionID string, // CDP session ID to filter events by this session only.
	creds *[2]string, // Optional [username, password] pair. nil means cancel auth.
	done <-chan struct{}, // Close signal to gracefully shut down the pump.
) {
	for {
		select {
		case <-done:
			// Shutdown signal received — exit the pump goroutine.
			return
		case ev, ok := <-events:
			// Channel closed or event available.
			if !ok {
				// CDP connection closed the event channel — exit.
				return
			}
			// Filter to only Fetch.authRequired events for our session.
			if ev.Method != "Fetch.authRequired" || ev.SessionID != sessionID {
				continue
			}
			// Extract requestId — required for Fetch.continueWithAuth.
			requestID := extractEventString(ev.Params, "requestId")
			if requestID == "" {
				continue
			}
			// Build the auth response (ProvideCredentials or CancelAuth) and send it.
			_, _ = conn.SendRecv(
				"Fetch.continueWithAuth",
				buildAuthResponse(requestID, creds),
				sessionID,
				requestPausedTimeout,
			)
		}
	}
}

// runNetworkCapturePump is a long-running goroutine that listens for
// CDP Network.responseReceived events and captures the bodies of XHR and
// Fetch requests that return JSON or plain text. It accumulates up to
// netCaptureMaxBodies responses (capped at netCaptureMaxTotalBytes total size)
// and stores them in the provided sink slice (protected by sinkMu).
//
// This pump enables callers to receive the raw API responses that SPAs use
// to render content, alongside the final HTML snapshot. Responses are only
// captured if they meet minimum size and MIME-type criteria to avoid
// polluting the result with error pages or non-content payloads.
//
// The pump runs until done is closed.
func runNetworkCapturePump(
	conn *cdpConnection, // Active CDP connection for sending commands.
	events <-chan cdpEvent, // Channel receiving all CDP events for this session.
	sessionID string, // CDP session ID to filter events by this session only.
	sink *[]capturedNetworkResponse, // Pointer to shared slice where captured responses are appended.
	sinkMu *sync.Mutex, // Mutex protecting concurrent appends to sink.
	done <-chan struct{}, // Close signal to gracefully shut down the pump.
) {
	totalBytes := 0 // Cumulative bytes across all captured responses; stops capture at limit.

	for {
		select {
		case <-done:
			// Shutdown signal received — exit the pump goroutine.
			return
		case ev, ok := <-events:
			// Channel closed or event available.
			if !ok {
				// CDP connection closed the event channel — exit.
				return
			}
			// Filter to Network.responseReceived events for our session.
			if ev.Method != "Network.responseReceived" || ev.SessionID != sessionID {
				continue
			}
			// Only capture XHR and Fetch request responses — skip document, script, img, etc.
			responseType := extractEventString(ev.Params, "type")
			if responseType != "XHR" && responseType != "Fetch" {
				continue
			}
			// Extract HTTP status code — only capture successful 2xx responses.
			status := extractNestedEventFloat(ev.Params, "response", "status")
			if status < 200 || status >= 300 {
				continue
			}
			// Extract Content-Type MIME type — only capture JSON and plain text.
			mime := extractNestedEventString(ev.Params, "response", "mimeType")
			if !isCapturableMIME(mime) {
				continue
			}
			// Check Content-Length header if present — skip very small bodies (often error pages).
			if contentLength := extractHeaderContentLength(ev.Params); contentLength > 0 && contentLength < netCaptureMinBodySize {
				continue
			}
			// Check global capture limits before fetching the body (avoids unnecessary CDP call).
			sinkMu.Lock()
			if len(*sink) >= netCaptureMaxBodies || totalBytes >= netCaptureMaxTotalBytes {
				sinkMu.Unlock()
				continue
			}
			sinkMu.Unlock()

			// Extract requestId needed to fetch the response body via CDP.
			requestID := extractEventString(ev.Params, "requestId")
			if requestID == "" {
				continue
			}
			// Send CDP command to retrieve the raw response body.
			// This may return base64-encoded content for binary responses.
			responseBody, err := conn.SendRecv(
				"Network.getResponseBody",
				map[string]any{"requestId": requestID},
				sessionID,
				netCaptureGetBodyTimeout,
			)
			if err != nil {
				// Timeout or connection error — skip this response.
				continue
			}
			// Parse the CDP response body payload.
			var bodyPayload map[string]any
			if err := json.Unmarshal(responseBody, &bodyPayload); err != nil {
				continue
			}
			// Extract the body string and base64 flag from the parsed payload.
			body, _ := bodyPayload["body"].(string)
			base64Encoded, _ := bodyPayload["base64Encoded"].(bool)
			// Skip base64-encoded bodies and bodies below minimum size threshold.
			if base64Encoded || len(body) < netCaptureMinBodySize {
				continue
			}
			// Copy values so they are safe to reference after the append below.
			mimeCopy := mime
			bodyCopy := body
			item := capturedNetworkResponse{
				URL:           extractNestedEventString(ev.Params, "response", "url"),
				RequestID:     requestID,
				Status:        uint16(status),
				MimeType:      &mimeCopy,
				BodySizeBytes: len(body),
				Body:          &bodyCopy,
			}
			// Append to shared slice — must hold mutex for both write and byte counter update.
			sinkMu.Lock()
			*sink = append(*sink, item)
			totalBytes += item.BodySizeBytes
			sinkMu.Unlock()
		}
	}
}

// extractEventString unmarshals a raw CDP event JSON payload and extracts
// a string field by key. Returns empty string if the key is missing, the
// value is not a string, or JSON parsing fails.
func extractEventString(raw json.RawMessage, key string) string {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "" // Malformed JSON — return empty to signal "not found".
	}
	val, _ := payload[key].(string)
	return val
}

// extractNestedEventString unmarshals a raw CDP event JSON payload, then
// navigates into a nested map (identified by parent key) and extracts a
// string field by key. Returns empty string on any failure.
func extractNestedEventString(raw json.RawMessage, parent, key string) string {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "" // Malformed JSON.
	}
	nested, _ := payload[parent].(map[string]any)
	if nested == nil {
		return "" // Parent key missing or not a map.
	}
	val, _ := nested[key].(string)
	return val
}

// extractNestedEventFloat unmarshals a raw CDP event JSON payload, navigates
// into a nested map (identified by parent key), and extracts a float64 field.
// CDP encodes integer status codes as JSON numbers, so float64 is used.
// Returns 0 on any failure.
func extractNestedEventFloat(raw json.RawMessage, parent, key string) float64 {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0
	}
	nested, _ := payload[parent].(map[string]any)
	if nested == nil {
		return 0
	}
	val, _ := nested[key].(float64)
	return val
}

// extractHeaderContentLength parses the Content-Length header value from a
// CDP Network.responseReceived event's response.headers map. Returns 0 if
// the header is missing or cannot be parsed as a positive integer.
func extractHeaderContentLength(raw json.RawMessage) int {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0 // Malformed JSON.
	}
	// Navigate to response.headers map.
	response, _ := payload["response"].(map[string]any)
	if response == nil {
		return 0
	}
	headers, _ := response["headers"].(map[string]any)
	if headers == nil {
		return 0
	}
	// Check both canonical and lowercase forms of Content-Length.
	for _, key := range []string{"Content-Length", "content-length"} {
		if v, ok := headers[key]; ok {
			if s, ok := v.(string); ok {
				if n, err := parsePositiveInt(s); err == nil {
					return n
				}
			}
		}
	}
	return 0
}

// parsePositiveInt parses a string as a non-negative integer using fmt.Sscanf.
// Returns an error if the string is not a valid non-negative integer.
func parsePositiveInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
	if err != nil || n < 0 {
		return 0, err
	}
	return n, nil
}
