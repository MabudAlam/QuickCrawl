package core

import (
	"strings"

	"github.com/MabudAlam/quickcrawl/internal/types"
)

func containsFormat(formats []types.OutputFormat, target types.OutputFormat) bool {
	for _, f := range formats {
		if f == target {
			return true
		}
	}
	return false
}

func includesJSONFormat(formats []types.OutputFormat) bool {
	return containsFormat(formats, types.FormatJson)
}

func buildFetchWarning(result *FetchResult) *string {
	if result.Warning != nil {
		return result.Warning
	}
	if result.StatusCode >= 400 {
		w := strings.TrimSpace(result.HTML)
		if len(w) > 100 {
			w = w[:100]
		}
		return &w
	}
	return detectBlockInterstitial(result.HTML)
}

func detectBlockInterstitial(html string) *string {
	if html == "" {
		return nil
	}
	lower := strings.ToLower(html)
	markers := []string{
		"just a moment",
		"attention required",
		"cf-browser-verification",
		"cf-challenge",
		"captcha",
		"access denied",
	}
	for _, m := range markers {
		if strings.Contains(lower, m) {
			msg := "blocked by anti-bot protection"
			return &msg
		}
	}
	return nil
}
