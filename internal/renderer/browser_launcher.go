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

// managedBrowser represents a browser process that we control.
type managedBrowser struct {
	cmd         *exec.Cmd // The browser process
	userDataDir string    // Temporary user data directory (Chrome)
	wsURL       string    // WebSocket URL for CDP connection
	browserName string    // Browser name (lightpanda, chrome)
}

// Close terminates the browser process and cleans up temporary files.
func (m *managedBrowser) Close() {
	if m == nil {
		return
	}
	// Kill the browser process
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
		_, _ = m.cmd.Process.Wait()
	}
	// Remove temporary user data directory
	if m.userDataDir != "" {
		_ = os.RemoveAll(m.userDataDir)
	}
}

// Name returns the browser name (lightpanda, chrome, etc).
func (m *managedBrowser) Name() string {
	if m == nil || m.browserName == "" {
		if m != nil && m.wsURL != "" && strings.Contains(m.wsURL, "lightpanda") {
			return "lightpanda"
		}
		return "unknown"
	}
	return m.browserName
}

// launchAllBrowsers attempts to launch both LightPanda and Chrome concurrently.
// It returns successfully launched browsers. If no browsers can be launched,
// an error is returned.
func launchAllBrowsers(rendererCfg *types.RendererConfig) ([]*managedBrowser, error) {
	type result struct {
		browser     *managedBrowser
		err         error
		browserName string
	}

	results := make(chan result, 2)

	// Launch LightPanda in background
	go func() {
		browser, err := tryLightPandaNative()
		if browser != nil {
			browser.browserName = "lightpanda"
		}
		results <- result{browser: browser, err: err, browserName: "lightpanda"}
	}()

	// Launch Chrome in background
	go func() {
		browser, err := tryChromeNative(rendererCfg)
		if browser != nil {
			browser.browserName = "chrome"
		}
		results <- result{browser: browser, err: err, browserName: "chrome"}
	}()

	// Collect successful launches
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

// tryLightPandaNative launches a native LightPanda browser instance.
// It finds or downloads the LightPanda binary, starts the serve process,
// and returns a managedBrowser with the WebSocket URL for CDP connection.
func tryLightPandaNative() (*managedBrowser, error) {
	// Find or download the LightPanda binary
	binary, err := findOrDownloadLightPanda()
	if err != nil {
		return nil, err
	}

	// Find an available port for CDP
	port, err := findAvailablePort()
	if err != nil {
		return nil, err
	}

	// Start LightPanda as a server
	cmd := exec.Command(binary, "serve", "--host", "127.0.0.1", "--port", fmt.Sprintf("%d", port))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Wait for CDP endpoint to become available
	wsURL, err := pollCDPEndpoint(port, 5*time.Second)
	if err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return nil, err
	}

	return &managedBrowser{
		cmd:   cmd,
		wsURL: wsURL,
	}, nil
}

// tryChromeNative launches a native Chrome browser instance.
// If wsURL is pre-configured in renderer config, it uses that URL.
// Otherwise, it locates the Chrome binary and spawns a headless browser.
func tryChromeNative(rendererCfg *types.RendererConfig) (*managedBrowser, error) {
	wsURL := ""
	if rendererCfg != nil {
		wsURL = getChromeWSURL(rendererCfg)
	}

	// If wsURL is pre-configured, return without launching Chrome
	if wsURL != "" {
		return &managedBrowser{wsURL: wsURL, browserName: "chrome"}, nil
	}

	// Find Chrome binary
	binary, err := findChromeBinary(rendererCfg)
	if err != nil {
		return nil, err
	}

	// Create temporary user data directory
	userDataDir, err := os.MkdirTemp("", "quickcrawl-chrome-*")
	if err != nil {
		return nil, err
	}

	// Chrome command-line arguments
	args := []string{
		"--headless=new",
		"--disable-gpu",
		"--no-sandbox",
		"--disable-dev-shm-usage",
		"--remote-debugging-port=0",
		"--remote-allow-origins=*",
	}

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

	// Read WebSocket URL from stderr
	wsURL, err = readWSURLFromStderrReader(stderr, cmd, 10*time.Second)
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

// findChromeBinary locates a Chrome or Chromium binary on the system.
// It checks the BrowserBinary config first, then searches common installation paths.
func findChromeBinary(rendererCfg *types.RendererConfig) (string, error) {
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

	// Common installation paths
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

// lightpandaManagedPath returns the path to the managed LightPanda binary.
// The binary is stored in ~/.quickcrawl/lightpanda.
func lightpandaManagedPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", fmt.Errorf("cannot find home directory")
	}
	return filepath.Join(home, ".quickcrawl", "lightpanda"), nil
}

// lightpandaDownloadURL returns the download URL for the current platform.
// It supports macOS ARM64, Linux AMD64, and Linux ARM64.
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

// findOrDownloadLightPanda finds or downloads the LightPanda binary.
// It first checks if lightpanda is available in PATH, then checks the managed
// installation location (~/.quickcrawl/lightpanda), and finally downloads if needed.
func findOrDownloadLightPanda() (string, error) {
	// Check if already in PATH
	if path := findInPath("lightpanda"); path != "" {
		return path, nil
	}

	// Check managed installation location
	managedPath, err := lightpandaManagedPath()
	if err != nil {
		return "", fmt.Errorf("lightpanda binary not found")
	}

	if _, err := os.Stat(managedPath); err == nil {
		return managedPath, nil
	}

	// Download if not found
	downloadURL := lightpandaDownloadURL()
	if downloadURL == "" {
		return "", fmt.Errorf("lightpanda binary not found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	// Create directory
	if err := os.MkdirAll(filepath.Dir(managedPath), 0o755); err != nil {
		return "", fmt.Errorf("failed to create ~/.quickcrawl directory: %w", err)
	}

	// Download binary
	output, err := exec.Command("curl", "-fsSL", "-o", managedPath, downloadURL).CombinedOutput()
	if err != nil {
		_ = os.Remove(managedPath)
		return "", fmt.Errorf("failed to download lightpanda: %s", string(output))
	}

	// Make executable
	if err := os.Chmod(managedPath, 0o755); err != nil {
		_ = os.Remove(managedPath)
		return "", fmt.Errorf("failed to chmod lightpanda: %w", err)
	}

	return managedPath, nil
}

// findAvailablePort finds a free port on localhost.
func findAvailablePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// pollCDPEndpoint polls the CDP version endpoint until WebSocket URL is available.
// It takes a port number and timeout duration, returning the WebSocket URL
// or an error if the timeout is reached.
func pollCDPEndpoint(port int, timeout time.Duration) (string, error) {
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

// readWSURLFromStderrReader reads the WebSocket URL from Chrome's stderr output.
// Chrome outputs "DevTools listening on ws://..." which we parse to extract the URL.
// It returns the WebSocket URL or an error if not found within the timeout.
func readWSURLFromStderrReader(stderr io.ReadCloser, cmd *exec.Cmd, timeout time.Duration) (string, error) {
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

// findInPath searches for a command in the system PATH.
// It returns the full path to the command if found, or an empty string.
func findInPath(name string) string {
	output, err := exec.Command("which", name).Output()
	if err != nil {
		return ""
	}
	if len(output) > 0 && len(strings.TrimSpace(string(output))) > 0 {
		return strings.TrimSpace(string(output))
	}
	return ""
}
