package config

import (
	"os"
	"path/filepath"
	"testing"
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
