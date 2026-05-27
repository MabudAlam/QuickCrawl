package renderer

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/types"
)

// managedBrowser represents a browser process that this package launched.
type managedBrowser struct {
	cmd         *exec.Cmd
	userDataDir string
	wsURL       string
	browserName string
}

// Close terminates the browser process and deletes any temporary data directory.
func (m *managedBrowser) Close() {
	if m == nil {
		return
	}
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
		_, _ = m.cmd.Process.Wait()
	}
	if m.userDataDir != "" {
		_ = os.RemoveAll(m.userDataDir)
	}
}

// Name returns the browser backend name for logging and diagnostics.
func (m *managedBrowser) Name() string {
	if m == nil || m.browserName == "" {
		if m != nil && m.wsURL != "" && strings.Contains(m.wsURL, "lightpanda") {
			return "lightpanda"
		}
		return "unknown"
	}
	return m.browserName
}

// launchAvailableBrowserBackends tries to start every supported native browser backend.
func launchAvailableBrowserBackends(rendererCfg *types.RendererConfig) ([]*managedBrowser, error) {
	type result struct {
		browser *managedBrowser
		err     error
	}

	results := make(chan result, 2)

	// Start both supported browsers in parallel so the first available backend wins.
	go func() {
		browser, err := startLightPandaBrowser()
		if browser != nil {
			browser.browserName = "lightpanda"
		}
		results <- result{browser: browser, err: err}
	}()

	go func() {
		browser, err := startChromeBrowser(rendererCfg)
		if browser != nil {
			browser.browserName = "chrome"
		}
		results <- result{browser: browser, err: err}
	}()

	var browsers []*managedBrowser
	for i := 0; i < 2; i++ {
		r := <-results
		if r.browser != nil {
			browsers = append(browsers, r.browser)
		}
	}

	if len(browsers) == 0 {
		return nil, fmt.Errorf("no browsers available")
	}

	return browsers, nil
}

// startLightPandaBrowser launches LightPanda locally and waits for its CDP endpoint.
func startLightPandaBrowser() (*managedBrowser, error) {
	// LightPanda exposes CDP over HTTP after it starts serving.
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

	return &managedBrowser{cmd: cmd, wsURL: wsURL}, nil
}

// startChromeBrowser launches Chrome locally unless the config already pins a WS URL.
func startChromeBrowser(rendererCfg *types.RendererConfig) (*managedBrowser, error) {
	wsURL := ""
	if rendererCfg != nil {
		wsURL = getChromeWSURL(rendererCfg)
	}

	if wsURL != "" {
		return &managedBrowser{wsURL: wsURL, browserName: "chrome"}, nil
	}

	binary, err := locateChromeBinary(rendererCfg)
	if err != nil {
		return nil, err
	}

	userDataDir, err := os.MkdirTemp("", "quickcrawl-chrome-*")
	if err != nil {
		return nil, err
	}

	args := []string{
		"--headless=new",
		"--disable-gpu",
		"--no-sandbox",
		"--disable-dev-shm-usage",
		"--remote-debugging-port=0",
		"--remote-allow-origins=*",
	}

	// Chrome writes the DevTools URL to stderr after startup.
	cmd := exec.Command(binary, args...)
	cmd.Stdout = os.Stdout
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = os.RemoveAll(userDataDir)
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(userDataDir)
		return nil, err
	}

	wsURL, err = readWebSocketURLFromStderr(stderr, 10*time.Second)
	if err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = os.RemoveAll(userDataDir)
		return nil, err
	}

	return &managedBrowser{
		cmd:         cmd,
		userDataDir: userDataDir,
		wsURL:       wsURL,
	}, nil
}

// locateChromeBinary resolves the Chrome/Chromium binary path from config or common locations.
func locateChromeBinary(rendererCfg *types.RendererConfig) (string, error) {
	browserBinary := ""
	if rendererCfg != nil {
		browserBinary = rendererCfg.BrowserBinary
	}

	if browserBinary != "" {
		if path, err := exec.LookPath(browserBinary); err == nil {
			return path, nil
		}
		if _, err := os.Stat(browserBinary); err == nil {
			return browserBinary, nil
		}
	}

	candidates := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
		"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		"google-chrome",
		"google-chrome-stable",
		"chromium",
		"chromium-browser",
	}

	for _, candidate := range candidates {
		if strings.HasPrefix(candidate, "/") {
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		} else {
			if path, err := exec.LookPath(candidate); err == nil {
				return path, nil
			}
		}
	}

	return "", fmt.Errorf("no browser binary found")
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
			}
			resp.Body.Close()
		}
		time.Sleep(200 * time.Millisecond)
	}

	return "", fmt.Errorf("browser did not expose CDP at %s within %s", url, timeout)
}

// readWebSocketURLFromStderr extracts Chrome's DevTools URL from stderr output.
func readWebSocketURLFromStderr(stderr io.ReadCloser, timeout time.Duration) (string, error) {
	defer stderr.Close()

	buf := make([]byte, 4096)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		n, err := stderr.Read(buf)
		if err != nil && err.Error() != "EOF" {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if n > 0 {
			output := string(buf[:n])
			if idx := strings.Index(output, "DevTools listening on "); idx >= 0 {
				wsURL := strings.TrimSpace(output[idx+len("DevTools listening on "):])
				if wsURL != "" {
					return wsURL, nil
				}
			}
			if idx := strings.Index(output, "ws://"); idx >= 0 {
				return strings.TrimSpace(output[idx:]), nil
			}
		}
		if err != nil && err.Error() == "EOF" {
			break
		}
	}

	return "", fmt.Errorf("DevTools WS URL not found in output within %s", timeout)
}

// lookPath returns the executable path for a command, if present.
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
