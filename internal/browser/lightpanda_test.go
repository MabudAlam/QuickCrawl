package browser

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestFindAvailableLocalPort(t *testing.T) {
	port, err := findAvailableLocalPort()
	if err != nil {
		t.Fatalf("findAvailableLocalPort: %v", err)
	}
	if port <= 0 || port > 65535 {
		t.Errorf("port %d is not in the valid range", port)
	}
}

func TestWaitForCDPEndpointURL_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"webSocketDebuggerUrl": "ws://127.0.0.1:9222/devtools/browser/abc",
		})
	}))
	defer srv.Close()

	port := extractPort(srv.URL)
	got, err := waitForCDPEndpointURL(port, 3*time.Second)
	if err != nil {
		t.Fatalf("waitForCDPEndpointURL: %v", err)
	}
	want := "ws://127.0.0.1:9222/devtools/browser/abc"
	if got != want {
		t.Errorf("waitForCDPEndpointURL = %q, want %q", got, want)
	}
}

func TestWaitForCDPEndpointURL_MissingField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"browser": "LightPanda/x"})
	}))
	defer srv.Close()

	port := extractPort(srv.URL)
	if _, err := waitForCDPEndpointURL(port, 500*time.Millisecond); err == nil {
		t.Error("expected timeout error when webSocketDebuggerUrl is missing, got nil")
	}
}

func TestWaitForCDPEndpointURL_EndpointDown(t *testing.T) {
	// Bind a port, immediately close it, then point waitForCDPEndpointURL
	// at the now-dead port. The poll loop should time out cleanly.
	port, err := findAvailableLocalPort()
	if err != nil {
		t.Fatalf("failed to allocate port: %v", err)
	}
	if _, err := waitForCDPEndpointURL(port, 500*time.Millisecond); err == nil {
		t.Error("expected timeout error against a closed port, got nil")
	}
}

func TestLightpandaDownloadURL(t *testing.T) {
	got := lightpandaDownloadURL()
	host := "https://github.com/lightpanda-io/browser/releases/download/nightly"
	switch {
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
		if got != host+"/lightpanda-aarch64-macos" {
			t.Errorf("unexpected URL for darwin/arm64: %q", got)
		}
	case runtime.GOOS == "linux" && runtime.GOARCH == "amd64":
		if got != host+"/lightpanda-x86_64-linux" {
			t.Errorf("unexpected URL for linux/amd64: %q", got)
		}
	case runtime.GOOS == "linux" && runtime.GOARCH == "arm64":
		if got != host+"/lightpanda-aarch64-linux" {
			t.Errorf("unexpected URL for linux/arm64: %q", got)
		}
	default:
		if got != "" {
			t.Errorf("expected empty URL for unsupported %s/%s, got %q", runtime.GOOS, runtime.GOARCH, got)
		}
	}
}

func TestLookPath_FindsCommand(t *testing.T) {
	if path, ok := lookPath("sh"); !ok || path == "" {
		t.Errorf("expected sh to be on PATH (path=%q ok=%v)", path, ok)
	}
}

func TestLookPath_MissingCommand(t *testing.T) {
	if path, ok := lookPath("definitely-not-a-real-binary-xyz"); ok || path != "" {
		t.Errorf("expected ok=false for missing binary (path=%q ok=%v)", path, ok)
	}
}

func TestLightpandaManagedPath(t *testing.T) {
	got, err := lightpandaManagedPath()
	if err != nil {
		t.Fatalf("lightpandaManagedPath: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("managed path should be absolute, got %q", got)
	}
	if filepath.Base(got) != "lightpanda" {
		t.Errorf("expected base name 'lightpanda', got %q", filepath.Base(got))
	}
}

func TestLightPandaLauncher_StopIdempotent(t *testing.T) {
	l := &LightPandaLauncher{}
	l.Stop()
	l.Stop()
	if !l.closed {
		t.Error("Stop did not set closed flag")
	}
}

func TestLightPandaLauncher_NilSafe(t *testing.T) {
	var l *LightPandaLauncher
	if got := l.WSURL(); got != "" {
		t.Errorf("nil launcher WSURL = %q, want empty", got)
	}
	l.Stop()
}

func TestFindOrDownloadLightPandaBinary_ExistingOnPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PATH", tmp)
	binPath := filepath.Join(tmp, "lightpanda")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("failed to write fake binary: %v", err)
	}

	got, err := findOrDownloadLightPandaBinary()
	if err != nil {
		t.Fatalf("findOrDownloadLightPandaBinary: %v", err)
	}
	if got != binPath {
		t.Errorf("expected to find lightpanda on PATH at %q, got %q", binPath, got)
	}
}

func TestFindOrDownloadLightPandaBinary_NoPathNoHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH manipulation is awkward on Windows")
	}
	tmp := t.TempDir()
	t.Setenv("PATH", tmp)

	// Point HOME at a path that cannot exist to force the managed-path
	// branch to fail before the download is attempted.
	impossible := filepath.Join(tmp, "does", "not", "exist")
	t.Setenv("HOME", impossible)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", impossible)
	}

	if _, err := findOrDownloadLightPandaBinary(); err == nil {
		t.Error("expected error when neither PATH nor home yields a binary, got nil")
	}
}

// extractPort pulls the TCP port from an httptest server URL like
// "http://127.0.0.1:54321".
func extractPort(url string) int {
	for i := len(url) - 1; i >= 0; i-- {
		if url[i] == ':' {
			portStr := url[i+1:]
			p := 0
			for _, c := range portStr {
				if c < '0' || c > '9' {
					return 0
				}
				p = p*10 + int(c-'0')
			}
			return p
		}
	}
	return 0
}
