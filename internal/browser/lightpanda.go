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

const lightpandaPort = 9222

var globalLauncher *LightPandaLauncher
var launcherMu sync.Mutex

type LightPandaLauncher struct {
	cmd    *exec.Cmd
	wsURL  string
	mu     sync.RWMutex
	closed bool
}

func (l *LightPandaLauncher) WSURL() string {
	if l == nil {
		return ""
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.wsURL
}

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

func StartLightPanda() (*LightPandaLauncher, error) {
	launcherMu.Lock()
	defer launcherMu.Unlock()

	if globalLauncher != nil && !globalLauncher.closed {
		return globalLauncher, nil
	}

	binary, err := findOrDownloadLightPandaBinary()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(binary, "serve", "--host", "127.0.0.1", "--port", fmt.Sprintf("%d", lightpandaPort))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	wsURL, err := waitForCDPEndpointURL(lightpandaPort, 10*time.Second)
	if err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return nil, err
	}

	globalLauncher = &LightPandaLauncher{cmd: cmd, wsURL: wsURL}
	return globalLauncher, nil
}

func EnsureRenderer(cfg *types.AppConfig) (func(), error) {
	if cfg == nil {
		return nil, fmt.Errorf("EnsureRenderer: cfg is nil")
	}
	if strings.EqualFold(string(cfg.Renderer.RenderMode), "http") {
		return nil, nil
	}

	if cfg.Renderer.Chrome != nil && strings.TrimSpace(cfg.Renderer.Chrome.WSURL) != "" {
		return func() {}, nil
	}

	lp, err := StartLightPanda()
	if err != nil {
		return nil, fmt.Errorf("EnsureRenderer: LightPanda auto-start failed: %w", err)
	}

	if cfg.Renderer.Chrome == nil {
		cfg.Renderer.Chrome = &types.CdpEndpoint{WSURL: lp.WSURL()}
	} else {
		cfg.Renderer.Chrome.WSURL = lp.WSURL()
	}

	return StopLightPanda, nil
}

func StopLightPanda() {
	launcherMu.Lock()
	defer launcherMu.Unlock()
	if globalLauncher != nil && !globalLauncher.closed {
		globalLauncher.Stop()
		globalLauncher = nil
	}
	_ = killLightPandaOnPort(lightpandaPort)
}

func killLightPandaOnPort(port int) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	output, err := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", port)).Output()
	if err != nil {
		return nil
	}
	pids := strings.TrimSpace(string(output))
	if pids == "" {
		return nil
	}
	for _, pid := range strings.Fields(pids) {
		_ = exec.Command("kill", "-9", pid).Run()
	}
	return nil
}

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
