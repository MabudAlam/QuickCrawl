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

const (
	netCaptureMaxBodies      = 30
	netCaptureMaxTotalBytes  = 2_000_000
	netCaptureMinBodySize    = 512
	netCaptureGetBodyTimeout = 800 * time.Millisecond
	requestPausedTimeout     = 2 * time.Second
)

var defaultBlockedResourceTypes = []string{
	"Image",
	"Media",
	"Font",
	"Manifest",
	"WebSocket",
}

var defaultBlockedHosts = []string{
	"google-analytics.com",
	"googletagmanager.com",
	"doubleclick.net",
	"googleadservices.com",
	"googlesyndication.com",
	"hotjar.com",
	"segment.io",
	"segment.com",
	"amplitude.com",
	"mixpanel.com",
	"clarity.ms",
	"onetrust.com",
	"cookielaw.org",
	"criteo.com",
	"criteo.net",
	"taboola.com",
	"outbrain.com",
	"adsystem.com",
	"adservice.google.com",
	"scorecardresearch.com",
	"quantserve.com",
	"chartbeat.com",
	"nr-data.net",
	"newrelic.com",
}

type blockReason string

const (
	blockReasonResourceType blockReason = "resource_type"
	blockReasonHost         blockReason = "host"
)

type requestBlocklist struct {
	resourceTypes    map[string]struct{}
	hostSubstrings   []string
	blockStylesheets bool
}

func newDefaultRequestBlocklist() requestBlocklist {
	resourceTypes := make(map[string]struct{}, len(defaultBlockedResourceTypes))
	for _, resourceType := range defaultBlockedResourceTypes {
		resourceTypes[resourceType] = struct{}{}
	}
	hostSubstrings := make([]string, 0, len(defaultBlockedHosts))
	for _, host := range defaultBlockedHosts {
		hostSubstrings = append(hostSubstrings, strings.ToLower(host))
	}
	return requestBlocklist{
		resourceTypes:    resourceTypes,
		hostSubstrings:   hostSubstrings,
		blockStylesheets: false,
	}
}

func (b requestBlocklist) shouldBlock(resourceType, requestURL string) (blockReason, bool) {
	if _, ok := b.resourceTypes[resourceType]; ok {
		return blockReasonResourceType, true
	}
	if b.blockStylesheets && resourceType == "Stylesheet" {
		return blockReasonResourceType, true
	}
	if len(b.hostSubstrings) == 0 {
		return "", false
	}
	parsed, err := url.Parse(requestURL)
	if err != nil {
		return "", false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return "", false
	}
	for _, needle := range b.hostSubstrings {
		if strings.Contains(host, needle) {
			return blockReasonHost, true
		}
	}
	return "", false
}

type capturedNetworkResponse struct {
	URL           string
	RequestID     string
	Status        uint16
	MimeType      *string
	BodySizeBytes int
	Body          *string
}

func toCapturedNetworkResponses(in []capturedNetworkResponse) []types.CapturedNetworkResponse {
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

func isCapturableMIME(mime string) bool {
	m := strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	switch m {
	case "application/json", "application/ld+json", "application/vnd.api+json", "text/json", "text/plain":
		return true
	default:
		return false
	}
}

func buildAuthResponse(requestID string, creds *[2]string) map[string]any {
	if creds != nil {
		return map[string]any{
			"requestId": requestID,
			"authChallengeResponse": map[string]any{
				"response": "ProvideCredentials",
				"username": creds[0],
				"password": creds[1],
			},
		}
	}
	return map[string]any{
		"requestId": requestID,
		"authChallengeResponse": map[string]any{
			"response": "CancelAuth",
		},
	}
}

// runFetchInterceptionPump handles Fetch.requestPaused events.
func runFetchInterceptionPump(
	conn *cdpConnection,
	events <-chan cdpEvent,
	sessionID string,
	blocklist requestBlocklist,
	done <-chan struct{},
) {
	for {
		select {
		case <-done:
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if ev.Method != "Fetch.requestPaused" || ev.SessionID != sessionID {
				continue
			}
			requestID := extractEventString(ev.Params, "requestId")
			if requestID == "" {
				continue
			}
			resourceType := extractEventString(ev.Params, "resourceType")
			requestURL := extractNestedEventString(ev.Params, "request", "url")
			if reason, ok := blocklist.shouldBlock(resourceType, requestURL); ok {
				_, _ = conn.SendRecv(
					"Fetch.failRequest",
					map[string]any{
						"requestId":   requestID,
						"errorReason": "BlockedByClient",
					},
					sessionID,
					requestPausedTimeout,
				)
				_ = reason
				continue
			}
			_, _ = conn.SendRecv(
				"Fetch.continueRequest",
				map[string]any{"requestId": requestID},
				sessionID,
				requestPausedTimeout,
			)
		}
	}
}

// runFetchAuthPump handles Fetch.authRequired events.
func runFetchAuthPump(
	conn *cdpConnection,
	events <-chan cdpEvent,
	sessionID string,
	creds *[2]string,
	done <-chan struct{},
) {
	for {
		select {
		case <-done:
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if ev.Method != "Fetch.authRequired" || ev.SessionID != sessionID {
				continue
			}
			requestID := extractEventString(ev.Params, "requestId")
			if requestID == "" {
				continue
			}
			_, _ = conn.SendRecv(
				"Fetch.continueWithAuth",
				buildAuthResponse(requestID, creds),
				sessionID,
				requestPausedTimeout,
			)
		}
	}
}

// runNetworkCapturePump stores JSON/text XHR and Fetch responses.
func runNetworkCapturePump(
	conn *cdpConnection,
	events <-chan cdpEvent,
	sessionID string,
	sink *[]capturedNetworkResponse,
	sinkMu *sync.Mutex,
	done <-chan struct{},
) {
	totalBytes := 0
	for {
		select {
		case <-done:
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if ev.Method != "Network.responseReceived" || ev.SessionID != sessionID {
				continue
			}
			if extractEventString(ev.Params, "type") != "XHR" && extractEventString(ev.Params, "type") != "Fetch" {
				continue
			}
			status := extractNestedEventFloat(ev.Params, "response", "status")
			if status < 200 || status >= 300 {
				continue
			}
			mime := extractNestedEventString(ev.Params, "response", "mimeType")
			if !isCapturableMIME(mime) {
				continue
			}
			if contentLength := extractHeaderContentLength(ev.Params); contentLength > 0 && contentLength < netCaptureMinBodySize {
				continue
			}
			sinkMu.Lock()
			if len(*sink) >= netCaptureMaxBodies || totalBytes >= netCaptureMaxTotalBytes {
				sinkMu.Unlock()
				continue
			}
			sinkMu.Unlock()

			requestID := extractEventString(ev.Params, "requestId")
			if requestID == "" {
				continue
			}
			responseBody, err := conn.SendRecv(
				"Network.getResponseBody",
				map[string]any{"requestId": requestID},
				sessionID,
				netCaptureGetBodyTimeout,
			)
			if err != nil {
				continue
			}
			var bodyPayload map[string]any
			if err := json.Unmarshal(responseBody, &bodyPayload); err != nil {
				continue
			}
			body, _ := bodyPayload["body"].(string)
			base64Encoded, _ := bodyPayload["base64Encoded"].(bool)
			if base64Encoded || len(body) < netCaptureMinBodySize {
				continue
			}
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
			sinkMu.Lock()
			*sink = append(*sink, item)
			totalBytes += item.BodySizeBytes
			sinkMu.Unlock()
		}
	}
}

func extractEventString(raw json.RawMessage, key string) string {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	val, _ := payload[key].(string)
	return val
}

func extractNestedEventString(raw json.RawMessage, parent, key string) string {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	nested, _ := payload[parent].(map[string]any)
	val, _ := nested[key].(string)
	return val
}

func extractNestedEventFloat(raw json.RawMessage, parent, key string) float64 {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0
	}
	nested, _ := payload[parent].(map[string]any)
	val, _ := nested[key].(float64)
	return val
}

func extractHeaderContentLength(raw json.RawMessage) int {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0
	}
	response, _ := payload["response"].(map[string]any)
	headers, _ := response["headers"].(map[string]any)
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

func parsePositiveInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
	if err != nil || n < 0 {
		return 0, err
	}
	return n, nil
}
