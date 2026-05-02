// Package common provides shared utilities and types used across all modules.
// It contains error types, URL validation, constants, and helper functions.
package common

// Application-wide constants.
const (
	// MaxURLLength is the maximum allowed length for URLs in characters.
	// URLs longer than this will be rejected.
	MaxURLLength = 2048
)

// BuiltinUAPool is a list of real browser User-Agent strings.
// Used for stealth mode to make requests appear as regular browsers.
var BuiltinUAPool = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_7_2) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.2 Safari/605.1.15",
}

// GetBuiltinUAPool returns a copy of the built-in user agent pool.
// Returns a fresh copy to prevent accidental mutation.
func GetBuiltinUAPool() []string {
	return BuiltinUAPool
}
