package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MabudAlam/quickcrawl/internal/types"
)

func TestDiscoverChromeWSURL_StaleUUIDReplaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(chromeVersionResponse{
			WebSocketDebuggerURL: "ws://" + r.Host + "/devtools/browser/NEW-UUID-FROM-CHROME",
		})
	}))
	defer srv.Close()

	configured := strings.Replace(srv.URL, "http://", "ws://", 1) + "/devtools/browser/OLD-STALE-UUID"
	got := discoverChromeWSURL(configured)
	want := strings.Replace(srv.URL, "http://", "ws://", 1) + "/devtools/browser/NEW-UUID-FROM-CHROME"
	if got != want {
		t.Errorf("discoverChromeWSURL = %q, want %q", got, want)
	}
}

func TestDiscoverChromeWSURL_EndpointUnreachable(t *testing.T) {
	wsURL := "ws://127.0.0.1:1/devtools/browser/anything"
	if got := discoverChromeWSURL(wsURL); got != "" {
		t.Errorf("expected empty string on unreachable endpoint, got %q", got)
	}
}

func TestDiscoverChromeWSURL_BadStatusCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()
	wsURL := strings.Replace(srv.URL, "http://", "ws://", 1) + "/devtools/browser/abc"
	if got := discoverChromeWSURL(wsURL); got != "" {
		t.Errorf("expected empty string on 404, got %q", got)
	}
}

func TestDiscoverChromeWSURL_MalformedPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()
	wsURL := strings.Replace(srv.URL, "http://", "ws://", 1) + "/devtools/browser/abc"
	if got := discoverChromeWSURL(wsURL); got != "" {
		t.Errorf("expected empty string on malformed JSON, got %q", got)
	}
}

func TestDiscoverChromeWSURL_MissingField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"browser": "Chrome/x"})
	}))
	defer srv.Close()
	wsURL := strings.Replace(srv.URL, "http://", "ws://", 1) + "/devtools/browser/abc"
	if got := discoverChromeWSURL(wsURL); got != "" {
		t.Errorf("expected empty string when field is missing, got %q", got)
	}
}

func TestWSURLToHTTPBase(t *testing.T) {
	cases := []struct {
		in  string
		out string
		ok  bool
	}{
		{"ws://localhost:9222/devtools/browser/abc", "http://localhost:9222", true},
		{"WS://Localhost:9222", "http://Localhost:9222", true},
		{"wss://chrome.example.com/devtools", "https://chrome.example.com", true},
		{"http://localhost:9222/json", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := wsURLToHTTPBase(c.in)
		if got != c.out || ok != c.ok {
			t.Errorf("wsURLToHTTPBase(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.out, c.ok)
		}
	}
}

func TestNewScraperFromConfig_EmptyWSURLDisablesBrowser(t *testing.T) {
	cfg := &types.AppConfig{}
	cfg.Defaults()
	cfg.Renderer.Chrome = &types.CdpEndpoint{WSURL: ""}

	scraper, qErr := NewScraperFromConfig(cfg, nil)
	if qErr != nil {
		t.Fatalf("NewScraperFromConfig: %v", qErr.Message)
	}
	defer scraper.Close()

	if scraper.cfg.Browser.WSURL != "" {
		t.Errorf("expected empty WSURL when [renderer.chrome] ws_url is empty, got %q", scraper.cfg.Browser.WSURL)
	}
}

func TestNewScraperFromConfig_ConfiguredWSURLPropagates(t *testing.T) {
	cfg := &types.AppConfig{}
	cfg.Defaults()
	cfg.Renderer.Chrome = &types.CdpEndpoint{WSURL: "ws://127.0.0.1:1/devtools/browser/xyz"}

	scraper, qErr := NewScraperFromConfig(cfg, nil)
	if qErr != nil {
		t.Fatalf("NewScraperFromConfig: %v", qErr.Message)
	}
	defer scraper.Close()

	if scraper.cfg.Browser.WSURL != "ws://127.0.0.1:1/devtools/browser/xyz" {
		t.Errorf("expected configured WSURL to be used (or auto-discovered), got %q", scraper.cfg.Browser.WSURL)
	}
}
