// Package config handles application configuration from files and environment variables.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/MabudAlam/quickcrawl/internal/types"
)

// LoadAppConfig loads the application configuration.
// Configuration is loaded in order:
// 1. Defaults
// 2. TOML config file (if CONFIG env is set, or default quickcrawl.toml)
// 3. Environment variables (always override)
func LoadAppConfig() (*types.AppConfig, error) {
	cfg := &types.AppConfig{}
	cfg.Defaults()

	configFile := strings.TrimSpace(os.Getenv("CONFIG"))
	if configFile == "" {
		configFile = "quickcrawl.toml"
	}

	if err := decodeConfigFile(cfg, configFile, false); err != nil {
		return nil, err
	}

	applyEnvOverrides(cfg)
	return cfg, nil
}

// decodeConfigFile loads and parses a TOML configuration file.
func decodeConfigFile(cfg *types.AppConfig, path string, required bool) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && !required {
			return nil
		}
		return fmt.Errorf("load config %s: %w", filepath.Clean(path), err)
	}
	if _, err := toml.Decode(string(data), cfg); err != nil {
		return fmt.Errorf("parse config %s: %w", filepath.Clean(path), err)
	}
	return nil
}

// applyEnvOverrides applies environment variable overrides to the config.
// Uses the format: <SECTION>__<KEY> (double underscore).
func applyEnvOverrides(cfg *types.AppConfig) {
	// Server configuration
	if v := envString("SERVER__HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := envUint16("SERVER__PORT"); v != nil {
		cfg.Server.Port = *v
	}
	if v := envInt64("SERVER__REQUEST_TIMEOUT_SECS"); v != nil {
		cfg.Server.RequestTimeoutSecs = *v
	}
	if v := envUint64("SERVER__RATE_LIMIT_RPS"); v != nil {
		cfg.Server.RateLimitRPS = *v
	}

	// Renderer configuration
	if v := envString("RENDERER__MODE"); v != "" {
		cfg.Renderer.Mode = types.RendererMode(strings.ToLower(v))
	}
	if v := envInt64("RENDERER__PAGE_TIMEOUT_MS"); v != nil {
		cfg.Renderer.PageTimeoutMs = *v
	}
	if v := envInt("RENDERER__POOL_SIZE"); v != nil {
		cfg.Renderer.PoolSize = *v
	}
	if v := envBool("RENDERER__RENDER_JS_DEFAULT"); v != nil {
		cfg.Renderer.RenderJSDefault = v
	}
	if v := envBool("RENDERER__FORCE_JS"); v != nil {
		cfg.Renderer.RenderJSDefault = v
	}
	if v := envString("RENDERER__LIGHTPANDA__WS_URL"); v != "" {
		cfg.Renderer.Lightpanda = &types.CdpEndpoint{WSURL: v}
	}
	if v := envString("RENDERER__CHROME__WS_URL"); v != "" {
		cfg.Renderer.Chrome = &types.CdpEndpoint{WSURL: v}
	}

	// Crawler configuration
	if v := envInt("CRAWLER__MAX_CONCURRENCY"); v != nil {
		cfg.Crawler.MaxConcurrency = *v
	}
	if v := envFloat64("CRAWLER__REQUESTS_PER_SECOND"); v != nil {
		cfg.Crawler.RequestsPerSecond = *v
	}
	if v := envBool("CRAWLER__RESPECT_ROBOTS_TXT"); v != nil {
		cfg.Crawler.RespectRobotsTxt = *v
	}
	if v := envString("CRAWLER__USER_AGENT"); v != "" {
		cfg.Crawler.UserAgent = v
	}
	if v := envString("CRAWLER__USER_AGENT"); v != "" {
		cfg.Crawler.UserAgent = v
	}
	if v := envInt("CRAWLER__DEFAULT_MAX_DEPTH"); v != nil {
		cfg.Crawler.DefaultMaxDepth = *v
	}
	if v := envInt("CRAWLER__DEFAULT_MAX_PAGES"); v != nil {
		cfg.Crawler.DefaultMaxPages = *v
	}
	if v := envString("CRAWLER__PROXY"); v != "" {
		cfg.Crawler.Proxy = ptr(v)
	}
	if v := envInt64("CRAWLER__JOB_TTL_SECS"); v != nil {
		cfg.Crawler.JobTTLSecs = *v
	}
	if v := envBool("CRAWLER__STEALTH__ENABLED"); v != nil {
		cfg.Crawler.Stealth.Enabled = *v
	}
	if v := envFloat64("CRAWLER__STEALTH__JITTER_FACTOR"); v != nil {
		cfg.Crawler.Stealth.JitterFactor = *v
	}
	if v := envBool("CRAWLER__STEALTH__INJECT_HEADERS"); v != nil {
		cfg.Crawler.Stealth.InjectHeaders = *v
	}
	if v := envString("CRAWLER__STEALTH__STRATEGY"); v != "" {
		cfg.Crawler.Stealth.Strategy = v
	}
	if v := envCSV("CRAWLER__STEALTH__USER_AGENTS"); v != nil {
		cfg.Crawler.Stealth.UserAgents = v
	}

	// Extraction configuration
	if v := envString("EXTRACTION__DEFAULT_FORMAT"); v != "" {
		cfg.Extraction.DefaultFormat = v
	}
	if v := envBool("EXTRACTION__ONLY_MAIN_CONTENT"); v != nil {
		cfg.Extraction.OnlyMainContent = *v
	}
	if cfg.Extraction.LLM == nil {
		cfg.Extraction.LLM = &types.LLMConfig{}
	}
	if v := envString("EXTRACTION__LLM__API_KEY"); v != "" {
		cfg.Extraction.LLM.APIKey = v
	}
	if v := envString("EXTRACTION__LLM__PROVIDER"); v != "" {
		cfg.Extraction.LLM.Provider = v
	}
	if v := envString("EXTRACTION__LLM__MODEL"); v != "" {
		cfg.Extraction.LLM.Model = v
	}
	if v := envString("EXTRACTION__LLM__BASE_URL"); v != "" {
		cfg.Extraction.LLM.BaseURL = ptr(v)
	}
	if v := envUint32("EXTRACTION__LLM__MAX_TOKENS"); v != nil {
		cfg.Extraction.LLM.MaxTokens = *v
	}
	if v := envString("EXTRACTION__LLM__EXTRACTION_PROMPT"); v != "" {
		cfg.Extraction.LLM.ExtractionPrompt = v
	}
	if v := envString("EXTRACTION__LLM__RESPONSE_FORMAT"); v != "" {
		cfg.Extraction.LLM.ResponseFormat = v
	}
}

// envString reads an environment variable and trims whitespace.
func envString(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

// envBool parses a boolean environment variable.
func envBool(key string) *bool {
	v := envString(key)
	if v == "" {
		return nil
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return nil
	}
	return &parsed
}

// envInt parses an integer environment variable.
func envInt(key string) *int {
	v := envString(key)
	if v == "" {
		return nil
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return nil
	}
	return &parsed
}

// envInt64 parses an int64 environment variable.
func envInt64(key string) *int64 {
	v := envString(key)
	if v == "" {
		return nil
	}
	parsed, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

// envUint16 parses a uint16 environment variable.
func envUint16(key string) *uint16 {
	v := envString(key)
	if v == "" {
		return nil
	}
	parsed, err := strconv.ParseUint(v, 10, 16)
	if err != nil {
		return nil
	}
	value := uint16(parsed)
	return &value
}

// envUint32 parses a uint32 environment variable.
func envUint32(key string) *uint32 {
	v := envString(key)
	if v == "" {
		return nil
	}
	parsed, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return nil
	}
	value := uint32(parsed)
	return &value
}

// envUint64 parses a uint64 environment variable.
func envUint64(key string) *uint64 {
	v := envString(key)
	if v == "" {
		return nil
	}
	parsed, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

// envFloat64 parses a float64 environment variable.
func envFloat64(key string) *float64 {
	v := envString(key)
	if v == "" {
		return nil
	}
	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

// envCSV parses a comma-separated list environment variable.
func envCSV(key string) []string {
	v := envString(key)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// ptr returns a pointer to a value.
func ptr[T any](v T) *T {
	return &v
}
