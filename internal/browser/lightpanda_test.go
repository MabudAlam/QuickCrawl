package browser

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/types"
)

func TestWaitForCDPEndpointURL_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"webSocketDebuggerUrl": "ws://127.0.0.1:99999/devtools/browser/abc",
		})
	}))
	defer srv.Close()

	port := extractPort(srv.URL)
	got, err := waitForCDPEndpointURL(port, 3*time.Second)
	if err != nil {
		t.Fatalf("waitForCDPEndpointURL: %v", err)
	}
	want := "ws://127.0.0.1:99999/devtools/browser/abc"
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
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate port: %v", err)
	}
	ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
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
	if got := lookPath("sh"); got == "" {
		t.Errorf("expected sh to be on PATH, got empty")
	}
}

func TestLookPath_MissingCommand(t *testing.T) {
	if got := lookPath("definitely-not-a-real-binary-xyz"); got != "" {
		t.Errorf("expected empty for missing binary, got %q", got)
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
	if runtime.GOOS == "windows" {
		t.Skip("PATH manipulation is awkward on Windows")
	}
	tmp := t.TempDir()
	binPath := filepath.Join(tmp, "lightpanda")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("failed to write fake binary: %v", err)
	}
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

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

	impossible := filepath.Join(tmp, "does", "not", "exist")
	t.Setenv("HOME", impossible)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", impossible)
	}

	if _, err := findOrDownloadLightPandaBinary(); err == nil {
		t.Error("expected error when neither PATH nor home yields a binary, got nil")
	}
}

func TestEnsureRenderer_AlreadyConfigured(t *testing.T) {
	cfg := &types.AppConfig{}
	cfg.Defaults()
	cfg.Renderer.Chrome = &types.CdpEndpoint{WSURL: "ws://127.0.0.1:9222/devtools/browser/abc"}

	teardown, err := EnsureRenderer(cfg)
	if err != nil {
		t.Fatalf("EnsureRenderer with configured WS URL: %v", err)
	}
	if teardown == nil {
		t.Error("expected non-nil teardown when WS URL already configured")
	}
	if cfg.Renderer.Chrome.WSURL != "ws://127.0.0.1:9222/devtools/browser/abc" {
		t.Errorf("EnsureRenderer should not mutate configured WS URL, got %q", cfg.Renderer.Chrome.WSURL)
	}
}

func TestEnsureRenderer_NilConfig(t *testing.T) {
	if _, err := EnsureRenderer(nil); err == nil {
		t.Error("expected error for nil config, got nil")
	}
}

func TestLightPandaLauncher_StopKillsProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process group signals behave differently on Windows")
	}
	sleepCmd := exec.Command("sleep", "60")
	if err := sleepCmd.Start(); err != nil {
		t.Fatalf("failed to start sleep process: %v", err)
	}
	pid := sleepCmd.Process.Pid
	launcher := &LightPandaLauncher{cmd: sleepCmd}
	launcher.Stop()
	process, err := os.FindProcess(pid)
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}
	_, err = process.Wait()
	if err == nil {
		t.Error("expected process to be killed, but it is still running")
	}
	launcher.Stop()
}

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
