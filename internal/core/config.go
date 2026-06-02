package core

import (
	"time"
)

type Config struct {
	Browser BrowserConfig
	Pool    PoolConfig
}

type BrowserConfig struct {
	Mode            BrowserMode
	WSURL           string
	NumBrowsers     int
	PageTimeout     time.Duration
	PoolSize        int
	StealthEnabled  bool // When true, register anti-fingerprint JS on every page. When false, the call is skipped entirely.
}

type BrowserMode string

const (
	BrowserModeAuto     BrowserMode = "auto"
	BrowserModeChrome   BrowserMode = "chrome"
	BrowserModeHTTPOnly BrowserMode = "http"
)

type PoolConfig struct {
	Size    int
	PerHost int
}

func DefaultConfig() Config {
	return Config{
		Browser: BrowserConfig{
			Mode:        BrowserModeAuto,
			WSURL:       "ws://localhost:9222",
			NumBrowsers: 4,
			PageTimeout: 60 * time.Second,
			PoolSize:    10,
		},
		Pool: PoolConfig{
			Size:    4,
			PerHost: 10,
		},
	}
}
