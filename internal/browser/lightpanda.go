// Package browser provides helpers for launching browser processes whose
// Chrome DevTools Protocol (CDP) endpoints are then consumed by the
// chromedp-based scraper.
//
// The package is intentionally small. It exposes two things:
//
//  1. StartLightPanda — low-level launcher that finds (or downloads) a
//     LightPanda binary, starts it on a free local port, polls its
//     /json/version endpoint for the live webSocketDebuggerUrl, and
//     returns a LightPandaLauncher whose Stop is idempotent.
//
//  2. EnsureRenderer — convenience used by MCP and CLI entry points:
//     if cfg.Renderer.Chrome.WSURL is empty, auto-launch LightPanda
//     and patch the config so the scraper picks it up. The HTTP server
//     does NOT call this; in the server deployment model the user is
//     expected to supply [renderer.chrome].ws_url explicitly.
package browser

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/types"
)

// LightPandaLauncher owns the lifecycle of a locally-spawned LightPanda
// process. The zero value is not usable; construct via StartLightPanda.
//
// Safe for concurrent use of WSURL. Stop may be called multiple times
// (idempotent).
type LightPandaLauncher struct {
	cmd    *exec.Cmd
	wsURL  string
	mu     sync.RWMutex
	closed bool
}

// WSURL returns the live Chrome DevTools Protocol WebSocket URL that
// LightPanda is serving on. Empty string before Start completes or
// after Stop.
func (l *LightPandaLauncher) WSURL() string {
	if l == nil {
		return ""
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.wsURL
}

// Stop terminates the LightPanda process (if running) and waits for
// the OS to reap it. It is safe to call multiple times; subsequent
// calls are no-ops.
func (l *LightPandaLauncher) Stop() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	l.closed = true
	if l.cmd != nil && l.cmd.Process != nil {
		_ = l.cmd.Process.Kill()
		_, _ = l.cmd.Process.Wait()
	}
}

// StartLightPanda launches a local LightPanda process, waits for its
// CDP endpoint to become reachable, and returns a launcher whose
// WSURL is the live webSocketDebuggerUrl. The process is killed and
// reaped when Stop is called.
//
// If the lightpanda binary is not on $PATH, the function downloads
// the platform-appropriate release from the official nightly URL into
// ~/.quickcrawl/lightpanda and uses that. A failed download leaves
// the launcher stopped and returns the error so the caller can decide
// what to do.
func StartLightPanda() (*LightPandaLauncher, error) {
	binary, err := findOrDownloadLightPandaBinary()
	if err != nil {
		return nil, err
	}

	port, err := findAvailableLocalPort()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(binary, "serve", "--host", "127.0.0.1", "--port", fmt.Sprintf("%d", port))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	wsURL, err := waitForCDPEndpointURL(port, 5*time.Second)
	if err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return nil, err
	}

	return &LightPandaLauncher{cmd: cmd, wsURL: wsURL}, nil
}

// EnsureRenderer guarantees that cfg.Renderer.Chrome has a WS URL by
// the time it returns, auto-launching a local LightPanda when the
// user has not configured one. If a teardown function is returned,
// the caller MUST invoke it on shutdown to reap the launched process.
//
// Returns (nil, nil) when the user already configured a WS URL, or
// when render_mode is explicitly "http" (the scraper will never reach
// the browser) — nothing to do, nothing to clean up.
//
// The HTTP server does NOT call this helper. It is for MCP and CLI
// entry points only.
func EnsureRenderer(cfg *types.AppConfig) (func(), error) {
	if cfg == nil {
		return nil, fmt.Errorf("EnsureRenderer: cfg is nil")
	}
	if strings.EqualFold(string(cfg.Renderer.RenderMode), "http") {
		return nil, nil
	}
	if cfg.Renderer.Chrome != nil && strings.TrimSpace(cfg.Renderer.Chrome.WSURL) != "" {
		return nil, nil
	}

	lp, err := StartLightPanda()
	if err != nil {
		return nil, fmt.Errorf("EnsureRenderer: no Chrome WS URL configured and LightPanda auto-start failed: %w (hint: set [renderer.chrome] ws_url in quickcrawl.toml to point at a running Chrome)", err)
	}

	if cfg.Renderer.Chrome == nil {
		cfg.Renderer.Chrome = &types.CdpEndpoint{WSURL: lp.WSURL()}
	} else {
		cfg.Renderer.Chrome.WSURL = lp.WSURL()
	}

	return lp.Stop, nil
}

// findOrDownloadLightPandaBinary returns a LightPanda binary, downloading it if necessary.
func findOrDownloadLightPandaBinary() (string, error) {
	if path := lookPath("lightpanda"); path != "" {
		return path, nil
	}

	managedPath, err := lightpandaManagedPath()
	if err != nil {
		return "", fmt.Errorf("lightpanda binary not found")
	}

	if _, err := os.Stat(managedPath); err == nil {
		return managedPath, nil
	}

	downloadURL := lightpandaDownloadURL()
	if downloadURL == "" {
		return "", fmt.Errorf("lightpanda binary not found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	if err := os.MkdirAll(filepath.Dir(managedPath), 0o755); err != nil {
		return "", fmt.Errorf("failed to create ~/.quickcrawl directory: %w", err)
	}

	output, err := exec.Command("curl", "-fsSL", "-o", managedPath, downloadURL).CombinedOutput()
	if err != nil {
		_ = os.Remove(managedPath)
		return "", fmt.Errorf("failed to download lightpanda: %s", string(output))
	}

	if err := os.Chmod(managedPath, 0o755); err != nil {
		_ = os.Remove(managedPath)
		return "", fmt.Errorf("failed to chmod lightpanda: %w", err)
	}

	return managedPath, nil
}

func lightpandaManagedPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", fmt.Errorf("cannot find home directory")
	}
	return filepath.Join(home, ".quickcrawl", "lightpanda"), nil
}

func lightpandaDownloadURL() string {
	base := "https://github.com/lightpanda-io/browser/releases/download/nightly"
	switch {
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
		return base + "/lightpanda-aarch64-macos"
	case runtime.GOOS == "linux" && runtime.GOARCH == "amd64":
		return base + "/lightpanda-x86_64-linux"
	case runtime.GOOS == "linux" && runtime.GOARCH == "arm64":
		return base + "/lightpanda-aarch64-linux"
	default:
		return ""
	}
}

// findAvailableLocalPort asks the OS for a free localhost TCP port.
func findAvailableLocalPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// waitForCDPEndpointURL polls the browser /json/version endpoint until CDP is available.
func waitForCDPEndpointURL(port int, timeout time.Duration) (string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			var payload struct {
				WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil {
				resp.Body.Close()
				if payload.WebSocketDebuggerURL != "" {
					return payload.WebSocketDebuggerURL, nil
				}
			} else {
				resp.Body.Close()
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	return "", fmt.Errorf("lightpanda did not expose CDP at %s within %s", url, timeout)
}

// lookPath returns the executable path for a command, if present.
// Uses the system `which` command, matching the legacy launcher.
func lookPath(name string) string {
	output, err := exec.Command("which", name).Output()
	if err != nil {
		return ""
	}
	if len(output) > 0 && len(strings.TrimSpace(string(output))) > 0 {
		return strings.TrimSpace(string(output))
	}
	return ""
}
