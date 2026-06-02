// Package browser provides helpers for launching browser processes whose
// Chrome DevTools Protocol (CDP) endpoints are then consumed by the
// chromedp-based scraper. The package is intentionally small and only
// covers the auto-launch fallback path: when the server is configured
// without an explicit browser WS URL, MCP starts LightPanda locally,
// discovers its CDP endpoint, and uses that for the lifetime of the
// process.
//
// The HTTP server path does not use this package — it requires the
// user to point cfg.Renderer.Chrome.WSURL at an already-running Chrome
// (with WS URL auto-discovery on startup). The browser-launching
// responsibility lives here because it is only needed for environments
// (like MCP) where the user may not have Chrome available.
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
	"sync"
	"time"
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
// ~/.quickcrawl/lightpanda and uses that. The download is a one-shot
// best-effort: a failure leaves the launcher stopped and returns the
// error so the caller can decide what to do.
func StartLightPanda() (*LightPandaLauncher, error) {
	binary, err := findOrDownloadLightPandaBinary()
	if err != nil {
		return nil, err
	}

	port, err := findAvailableLocalPort()
	if err != nil {
		return nil, fmt.Errorf("lightpanda: failed to allocate port: %w", err)
	}

	cmd := exec.Command(binary, "serve", "--host", "127.0.0.1", "--port", fmt.Sprintf("%d", port))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lightpanda: failed to start process: %w", err)
	}

	wsURL, err := waitForCDPEndpointURL(port, 10*time.Second)
	if err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return nil, fmt.Errorf("lightpanda: %w", err)
	}

	return &LightPandaLauncher{cmd: cmd, wsURL: wsURL}, nil
}

// findOrDownloadLightPandaBinary returns a LightPanda binary path,
// preferring $PATH and falling back to a one-shot download into
// ~/.quickcrawl/lightpanda for the current OS/arch.
func findOrDownloadLightPandaBinary() (string, error) {
	if path, err := exec.LookPath("lightpanda"); err == nil && path != "" {
		return path, nil
	}

	managedPath, err := lightpandaManagedPath()
	if err != nil {
		return "", fmt.Errorf("lightpanda binary not found on PATH and home directory is unavailable: %w", err)
	}

	if _, err := os.Stat(managedPath); err == nil {
		return managedPath, nil
	}

	downloadURL := lightpandaDownloadURL()
	if downloadURL == "" {
		return "", fmt.Errorf("lightpanda binary not found and no download available for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	if err := os.MkdirAll(filepath.Dir(managedPath), 0o755); err != nil {
		return "", fmt.Errorf("lightpanda: failed to create %s: %w", filepath.Dir(managedPath), err)
	}

	output, err := exec.Command("curl", "-fsSL", "-o", managedPath, downloadURL).CombinedOutput()
	if err != nil {
		_ = os.Remove(managedPath)
		return "", fmt.Errorf("lightpanda: download from %s failed: %s", downloadURL, string(output))
	}

	if err := os.Chmod(managedPath, 0o755); err != nil {
		_ = os.Remove(managedPath)
		return "", fmt.Errorf("lightpanda: chmod failed: %w", err)
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

// findAvailableLocalPort asks the OS for a free localhost TCP port
// and returns it. The kernel does not reserve the port for us, so a
// race is possible in theory; the caller should bind it as soon as
// possible.
func findAvailableLocalPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// waitForCDPEndpointURL polls the browser /json/version endpoint until
// the CDP WebSocket URL is available, or until the timeout elapses.
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
			if decErr := json.NewDecoder(resp.Body).Decode(&payload); decErr == nil {
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

// lookPath returns the absolute path of an executable on $PATH, or
// the empty string if it is not present. Mirrors the legacy helper
// the original browser-process code used.
func lookPath(name string) (string, bool) {
	path, err := exec.LookPath(name)
	if err != nil || path == "" {
		return "", false
	}
	return path, true
}
