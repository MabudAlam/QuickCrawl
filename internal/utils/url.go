package utils

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

var privateCIDRs []*net.IPNet

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

func ValidateURL(urlStr string) (*url.URL, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if err := ValidateSafeURL(u); err != nil {
		return nil, err
	}

	return u, nil
}