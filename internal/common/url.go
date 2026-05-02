// Package common provides shared utilities and types used across all modules.
// It contains error types, URL validation, constants, and helper functions.
package common

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

var privateCIDRs []*net.IPNet

func init() {
	privateBlocks := []string{
		"127.0.0.0/8",    // Loopback (127.0.0.1 - 127.255.255.255)
		"10.0.0.0/8",     // Private Class A (10.0.0.0 - 10.255.255.255)
		"172.16.0.0/12",  // Private Class B (172.16.0.0 - 172.31.255.255)
		"192.168.0.0/16", // Private Class C (192.168.0.0 - 192.168.255.255)
		"169.254.0.0/16", // Link-local (169.254.0.0 - 169.254.255.255)
		"::1",            // IPv6 loopback
		"fc00::/7",       // IPv6 unique local
		"fe80::/10",      // IPv6 link-local
	}
	for _, block := range privateBlocks {
		_, cidr, _ := net.ParseCIDR(block)
		if cidr != nil {
			privateCIDRs = append(privateCIDRs, cidr)
		}
	}
}

// ValidateSafeURL checks if a parsed URL is safe to use for crawling.
// It validates:
//   - URL length does not exceed MaxURLLength
//   - No null bytes present
//   - Scheme is http or https
//   - Host is not empty
//   - Host is not localhost or private IP range
func ValidateSafeURL(u *url.URL) error {
	urlStr := u.String()

	if len(urlStr) > MaxURLLength {
		return fmt.Errorf("URL exceeds maximum length of %d characters", MaxURLLength)
	}

	if strings.Contains(urlStr, "\x00") || strings.Contains(urlStr, "%00") {
		return fmt.Errorf("URL contains null bytes")
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("Only http/https URLs are allowed")
	}

	host := u.Host
	if host == "" {
		return fmt.Errorf("URL has no host")
	}

	hostWithoutPort := host
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		if host[0] != '[' {
			hostWithoutPort = host[:idx]
		}
	}

	hostLower := strings.ToLower(hostWithoutPort)
	if hostLower == "localhost" || strings.HasSuffix(hostLower, ".localhost") {
		return fmt.Errorf("Localhost URLs are not allowed")
	}

	if ip := net.ParseIP(hostWithoutPort); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("Access to %s is not allowed", ip.String())
		}
	}

	stripped := strings.TrimPrefix(strings.TrimPrefix(host, "["), "]")
	if ip := net.ParseIP(stripped); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("Access to %s is not allowed", ip.String())
		}
	}

	return nil
}

// isBlockedIP checks if an IP address falls within private or reserved ranges.
// Returns true if the IP should be blocked (private network access).
func isBlockedIP(ip net.IP) bool {
	for _, cidr := range privateCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}

	if ipv4 := ip.To4(); ipv4 != nil {
		return isBlockedIP(ipv4)
	}

	return false
}

// ValidateURL parses a URL string and validates it's safe for crawling.
// Returns a parsed URL on success or an error if the URL is invalid or unsafe.
func ValidateURL(urlStr string) (*url.URL, error) {
	// Parse the raw URL string
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Run safety checks on the parsed URL
	if err := ValidateSafeURL(u); err != nil {
		return nil, err
	}

	return u, nil
}
