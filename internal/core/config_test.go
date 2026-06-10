package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MabudAlam/quickcrawl/internal/types"
)

func TestGetCDPURL_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(VersionResponse{
			WebSocketDebuggerURL: "wss://" + r.Host + "/devtools/browser/NEW-UUID-FROM-CHROME",
		})
	}))
	defer srv.Close()

	wsURL := strings.Replace(srv.URL, "http://", "ws://", 1)
	got, err := GetCDPURL(wsURL)
	want := "wss://" + srv.Listener.Addr().String() + "/devtools/browser/NEW-UUID-FROM-CHROME"
	if err != nil {
		t.Fatalf("GetCDPURL returned error: %v", err)
	}
	if got != want {
		t.Errorf("GetCDPURL = %q, want %q", got, want)
	}
}

func TestGetCDPURL_EndpointUnreachable(t *testing.T) {
	_, err := GetCDPURL("http://127.0.0.1:1")
	if err == nil {
		t.Errorf("expected error on unreachable endpoint")
	}
}

func TestGetCDPURL_BadStatusCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := GetCDPURL(srv.URL)
	if err == nil {
		t.Errorf("expected error on 404")
	}
}

func TestGetCDPURL_MalformedPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()
	_, err := GetCDPURL(srv.URL)
	if err == nil {
		t.Errorf("expected error on malformed JSON")
	}
}

func TestGetCDPURL_MissingField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"browser": "Chrome/x"})
	}))
	defer srv.Close()
	_, err := GetCDPURL(srv.URL)
	if err == nil {
		t.Errorf("expected error when field is missing")
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
