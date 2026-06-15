package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MabudAlam/quickcrawl/internal/types"
)

// writeFile is a tiny helper for the precedence tests.
func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestLoadAppConfig_LLMKey_FromTOML(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "qc.toml")
	writeFile(t, cfgPath, `
[extraction.llm]
api_key = "sk-toml-only"
`)

	cfg, err := LoadAppConfigFromPath(cfgPath)
	if err != nil {
		t.Fatalf("LoadAppConfigFromPath: %v", err)
	}
	if got := cfg.Extraction.LLM.APIKey; got != "sk-toml-only" {
		t.Errorf("expected TOML API key, got %q", got)
	}
}

func TestLoadAppConfig_LLMKey_EnvOverridesTOML(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "qc.toml")
	writeFile(t, cfgPath, `
[extraction.llm]
api_key = "sk-toml-should-lose"
`)

	t.Setenv("EXTRACTION__LLM__API_KEY", "sk-env-should-win")

	cfg, err := LoadAppConfigFromPath(cfgPath)
	if err != nil {
		t.Fatalf("LoadAppConfigFromPath: %v", err)
	}
	if got := cfg.Extraction.LLM.APIKey; got != "sk-env-should-win" {
		t.Errorf("env should override TOML, got %q", got)
	}
}

func TestLoadAppConfig_LLMKey_EmptyBoth(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "qc.toml")
	writeFile(t, cfgPath, `
[extraction.llm]
model = "gpt-4o-mini"
`)

	cfg, err := LoadAppConfigFromPath(cfgPath)
	if err != nil {
		t.Fatalf("LoadAppConfigFromPath: %v", err)
	}
	if got := cfg.Extraction.LLM.APIKey; got != "" {
		t.Errorf("expected empty API key when neither TOML nor env is set, got %q", got)
	}
}

// =============================================================================
// NewScraperFromConfig tests
//
// These were previously in internal/core/config_test.go. They live here
// now because NewScraperFromConfig moved to this package. CDP URL tests
// (TestGetCDPURL_*) stay in internal/core (as internal/core/cdp_test.go)
// because GetCDPURL is still a core function — see cdp.go.
// =============================================================================

func TestNewScraperFromConfig_EmptyWSURLDisablesBrowser(t *testing.T) {
	cfg := &types.AppConfig{}
	cfg.Defaults()
	cfg.Renderer.Chrome = &types.CdpEndpoint{WSURL: ""}

	scraper, qErr := NewScraperFromConfig(cfg, nil)
	if qErr != nil {
		t.Fatalf("NewScraperFromConfig: %v", qErr.Message)
	}
	defer scraper.Close()

	if scraper.Config().Browser.WSURL != "" {
		t.Errorf("expected empty WSURL when [renderer.chrome] ws_url is empty, got %q", scraper.Config().Browser.WSURL)
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

	if scraper.Config().Browser.WSURL != "ws://127.0.0.1:1/devtools/browser/xyz" {
		t.Errorf("expected configured WSURL to be used (or auto-discovered), got %q", scraper.Config().Browser.WSURL)
	}
}

func TestNewScraperFromConfig_RenderModeDefaultsToAuto(t *testing.T) {
	cfg := &types.AppConfig{}
	cfg.Defaults()

	scraper, qErr := NewScraperFromConfig(cfg, nil)
	if qErr != nil {
		t.Fatalf("NewScraperFromConfig: %v", qErr.Message)
	}
	defer scraper.Close()

	if scraper.Config().Browser.Mode != types.RenderModeAuto {
		t.Errorf("expected default Mode to be RenderModeAuto, got %q", scraper.Config().Browser.Mode)
	}
}

func TestNewScraperFromConfig_RenderModeOverridesMode(t *testing.T) {
	cases := []struct {
		name string
		set  types.RenderMode
		want types.RenderMode
	}{
		{"auto", types.RenderModeAuto, types.RenderModeAuto},
		{"browser", types.RenderModeBrowser, types.RenderModeBrowser},
		{"http", types.RenderModeHTTP, types.RenderModeHTTP},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &types.AppConfig{}
			cfg.Defaults()
			cfg.Renderer.RenderMode = tc.set

			scraper, qErr := NewScraperFromConfig(cfg, nil)
			if qErr != nil {
				t.Fatalf("NewScraperFromConfig: %v", qErr.Message)
			}
			defer scraper.Close()

			if scraper.Config().Browser.Mode != tc.want {
				t.Errorf("render_mode=%q: got Mode=%q, want %q", tc.set, scraper.Config().Browser.Mode, tc.want)
			}
		})
	}
}

func TestNewScraperFromConfig_InvalidRenderModeRejected(t *testing.T) {
	cfg := &types.AppConfig{}
	cfg.Defaults()
	cfg.Renderer.RenderMode = types.RenderMode("always-sparkle")

	_, qErr := NewScraperFromConfig(cfg, nil)
	if qErr == nil {
		t.Fatal("expected error for invalid render_mode, got nil")
	}
}

func TestParseRenderMode(t *testing.T) {
	cases := []struct {
		in   string
		want types.RenderMode
		err  bool
	}{
		{"", "", false},
		{"auto", types.RenderModeAuto, false},
		{"browser", types.RenderModeBrowser, false},
		{"http", types.RenderModeHTTP, false},
		{"  AUTO  ", types.RenderModeAuto, false},
		{"Browser", types.RenderModeBrowser, false},
		{"always-sparkle", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := types.ParseRenderMode(tc.in)
			if tc.err {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("ParseRenderMode(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
