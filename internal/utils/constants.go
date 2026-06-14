package utils

import (
	"time"
)

const (
	MaxResponseBytes   = 10 * 1024 * 1024
	HTTPConnectTimeout = 5 * time.Second
	HTTPRequestTimeout = 30 * time.Second
	HTTPMaxRetries     = 1
	HTTPRetryBackoff   = 250 * time.Millisecond
	MaxURLLength       = 2048
)

var BuiltinUAPool = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_7_2) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.2 Safari/605.1.15",
}

func GetBuiltinUAPool() []string {
	return BuiltinUAPool
}