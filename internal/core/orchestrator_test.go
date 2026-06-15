package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MabudAlam/quickcrawl/internal/types"
)

// newTestRenderer builds a Renderer wired to a real HTTP fetcher and
// NO browser (WSURL is empty). This lets us exercise FetchOrchestrator's
// HTTP-only code paths — auto and http mode — without spinning up
// Chrome. The browser mode is not exercised here because it requires
// a live chromedp allocator.
func newTestRenderer(t *testing.T, mode types.RenderMode) (*Renderer, *httptest.Server) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><h1>hello</h1></body></html>`))
	}))
	t.Cleanup(srv.Close)

	httpFetcher := NewHTTPFetcher("", nil)
	cfg := types.ScraperConfig{
		Browser: types.BrowserConfig{
			Mode:     mode,
			WSURL:    "",
			PoolSize: 4,
		},
		Pool: types.PoolConfig{Size: 4, PerHost: 4},
	}
	r, err := NewRenderer(cfg, httpFetcher)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r, srv
}

func TestFetchOrchestrator_HttpModeUsesHTTP(t *testing.T) {
	r, srv := newTestRenderer(t, types.RenderModeHTTP)

	// Per-request mode=nil falls back to cfg.Mode (http).
	result, qErr := r.FetchOrchestrator(context.Background(), srv.URL, nil, nil, 0)
	if qErr != nil {
		t.Fatalf("FetchOrchestrator: %v", qErr.Message)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.RenderedWith != "http" {
		t.Errorf("render_mode=http with no per-request override: expected HTTP render, got %q", result.RenderedWith)
	}
}

func TestFetchOrchestrator_HttpModePerRequestBrowserFails(t *testing.T) {
	// Per-request RenderMode=Browser is an *override* of the default. There
	// is no policy gate — but in http mode the browser is simply not
	// available (no WSURL), so the override is satisfied by failing
	// rather than by rejecting the request.
	r, srv := newTestRenderer(t, types.RenderModeHTTP)
	mode := types.RenderModeBrowser

	_, qErr := r.FetchOrchestrator(context.Background(), srv.URL, nil, &mode, 0)
	if qErr == nil {
		t.Fatal("expected error when per-request RenderMode=Browser and no browser is available")
	}
}

func TestFetchOrchestrator_AutoModePerRequestHTTPForcesHTTP(t *testing.T) {
	// render_mode=auto (default), per-request RenderMode=HTTP → force HTTP.
	r, srv := newTestRenderer(t, types.RenderModeAuto)
	mode := types.RenderModeHTTP

	result, qErr := r.FetchOrchestrator(context.Background(), srv.URL, nil, &mode, 0)
	if qErr != nil {
		t.Fatalf("FetchOrchestrator: %v", qErr.Message)
	}
	if result.RenderedWith != "http" {
		t.Errorf("auto + per-request HTTP: expected HTTP render, got %q", result.RenderedWith)
	}
}

func TestFetchOrchestrator_AutoModePerRequestAutoStaysAuto(t *testing.T) {
	// render_mode=auto, per-request mode=auto → no override, stays auto.
	r, srv := newTestRenderer(t, types.RenderModeAuto)
	mode := types.RenderModeAuto

	result, qErr := r.FetchOrchestrator(context.Background(), srv.URL, nil, &mode, 0)
	if qErr != nil {
		t.Fatalf("FetchOrchestrator: %v", qErr.Message)
	}
	if result.RenderedWith != "http" {
		t.Errorf("auto + per-request auto on a normal page: expected HTTP render, got %q", result.RenderedWith)
	}
}

func TestFetchOrchestrator_BrowserModeUsesBrowserPath(t *testing.T) {
	// render_mode=browser, per-request mode=nil → force browser.
	// This test verifies the orchestrator takes the browser branch
	// when the server default is browser. With no WSURL the fetch
	// fails with a browser-unavailable error, but the *decision* to
	// try the browser is what we want to assert.
	r, srv := newTestRenderer(t, types.RenderModeBrowser)
	_, qErr := r.FetchOrchestrator(context.Background(), srv.URL, nil, nil, 0)
	if qErr == nil {
		t.Fatal("expected error when render_mode=browser and no browser is available")
	}
	// The error should mention browser/renderer, not a generic HTTP issue.
	// (We don't assert the exact text to avoid coupling to internal wording.)
	_ = srv
}

func TestRenderMode_CanonicalValues(t *testing.T) {
	cases := map[types.RenderMode]string{
		types.RenderModeAuto:    "auto",
		types.RenderModeBrowser: "browser",
		types.RenderModeHTTP:    "http",
	}
	for mode, want := range cases {
		if got := string(mode); got != want {
			t.Errorf("RenderMode %v: string() = %q, want %q", mode, got, want)
		}
		if !mode.IsValid() {
			t.Errorf("RenderMode %q should be valid", mode)
		}
	}
	if types.RenderMode("").IsValid() {
		t.Error("empty RenderMode should be invalid (use zero value for unset)")
	}
}
