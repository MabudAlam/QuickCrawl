package utils

import "math/rand"

// HeaderProfile represents a complete set of HTTP headers for a device/browser combination
type HeaderProfile struct {
	UserAgent       string
	Accept          string
	AcceptLanguage  string
	AcceptEncoding  string
	SecFetchDest    string
	SecFetchMode    string
	SecFetchSite    string
	SecFetchUser    string
	SecChUa         string
	SecChUaMobile   string
	SecChUaPlatform string
}

// HeaderStrategy represents different approach styles for scraping
type HeaderStrategy string

const (
	StrategyModernBrowser HeaderStrategy = "modern_browser"
	StrategyMobileDevice  HeaderStrategy = "mobile_device"
	StrategyBotFriendly   HeaderStrategy = "bot_friendly"
)

var modernBrowserProfiles = []HeaderProfile{
	{
		UserAgent:       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		Accept:          "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
		AcceptLanguage:  "en-US,en;q=0.9",
		AcceptEncoding:  "gzip, deflate, br, zstd",
		SecFetchDest:    "document",
		SecFetchMode:    "navigate",
		SecFetchSite:    "none",
		SecFetchUser:    "?1",
		SecChUa:         `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`,
		SecChUaMobile:   "?0",
		SecChUaPlatform: `"macOS"`,
	},
	{
		UserAgent:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		Accept:          "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
		AcceptLanguage:  "en-US,en;q=0.9",
		AcceptEncoding:  "gzip, deflate, br, zstd",
		SecFetchDest:    "document",
		SecFetchMode:    "navigate",
		SecFetchSite:    "none",
		SecFetchUser:    "?1",
		SecChUa:         `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`,
		SecChUaMobile:   "?0",
		SecChUaPlatform: `"Windows"`,
	},
	{
		UserAgent:       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.2 Safari/605.1.15",
		Accept:          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		AcceptLanguage:  "en-US,en;q=0.9",
		AcceptEncoding:  "gzip, deflate, br",
		SecFetchDest:    "document",
		SecFetchMode:    "navigate",
		SecFetchSite:    "none",
		SecFetchUser:    "",
		SecChUa:         "",
		SecChUaMobile:   "",
		SecChUaPlatform: "",
	},
}

var mobileDeviceProfiles = []HeaderProfile{
	{
		UserAgent:       "Mozilla/5.0 (iPhone; CPU iPhone OS 18_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.2 Mobile/15E148 Safari/604.1",
		Accept:          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		AcceptLanguage:  "en-US,en;q=0.9",
		AcceptEncoding:  "gzip, deflate, br",
		SecFetchDest:    "document",
		SecFetchMode:    "navigate",
		SecFetchSite:    "none",
		SecFetchUser:    "",
		SecChUa:         "",
		SecChUaMobile:   "?1",
		SecChUaPlatform: `"iOS"`,
	},
	{
		UserAgent:       "Mozilla/5.0 (iPhone; CPU iPhone OS 17_7 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.7 Mobile/15E148 Safari/604.1",
		Accept:          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		AcceptLanguage:  "en-US,en;q=0.9",
		AcceptEncoding:  "gzip, deflate, br",
		SecFetchDest:    "document",
		SecFetchMode:    "navigate",
		SecFetchSite:    "none",
		SecFetchUser:    "",
		SecChUa:         "",
		SecChUaMobile:   "?1",
		SecChUaPlatform: `"iOS"`,
	},
	{
		UserAgent:       "Mozilla/5.0 (iPad; CPU OS 18_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.2 Mobile/15E148 Safari/604.1",
		Accept:          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		AcceptLanguage:  "en-US,en;q=0.9",
		AcceptEncoding:  "gzip, deflate, br",
		SecFetchDest:    "document",
		SecFetchMode:    "navigate",
		SecFetchSite:    "none",
		SecFetchUser:    "",
		SecChUa:         "",
		SecChUaMobile:   "?1",
		SecChUaPlatform: `"iOS"`,
	},
	{
		UserAgent:       "Mozilla/5.0 (Linux; Android 14; Pixel 8 Pro) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36",
		Accept:          "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
		AcceptLanguage:  "en-US,en;q=0.9",
		AcceptEncoding:  "gzip, deflate, br, zstd",
		SecFetchDest:    "document",
		SecFetchMode:    "navigate",
		SecFetchSite:    "none",
		SecFetchUser:    "?1",
		SecChUa:         `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`,
		SecChUaMobile:   "?1",
		SecChUaPlatform: `"Android"`,
	},
}

var botFriendlyProfiles = []HeaderProfile{
	{
		UserAgent:       "SupacrawlerBot/1.0 (+https://supacrawler.com/bot)",
		Accept:          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		AcceptLanguage:  "en-US,en;q=0.9",
		AcceptEncoding:  "gzip, deflate, br",
		SecFetchDest:    "",
		SecFetchMode:    "",
		SecFetchSite:    "",
		SecFetchUser:    "",
		SecChUa:         "",
		SecChUaMobile:   "",
		SecChUaPlatform: "",
	},
	{
		UserAgent:       "Mozilla/5.0 (compatible; SupacrawlerBot/1.0; +https://supacrawler.com/bot)",
		Accept:          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		AcceptLanguage:  "en-US,en;q=0.9",
		AcceptEncoding:  "gzip, deflate, br",
		SecFetchDest:    "",
		SecFetchMode:    "",
		SecFetchSite:    "",
		SecFetchUser:    "",
		SecChUa:         "",
		SecChUaMobile:   "",
		SecChUaPlatform: "",
	},
}

var acceptLanguagePool = []string{
	"en-US,en;q=0.9",
	"en-GB,en;q=0.9",
	"en-AU,en;q=0.9",
	"en-CA,en;q=0.9",
	"de-DE,de;q=0.9,en;q=0.8",
	"de-AT,de;q=0.9,en;q=0.8",
	"de-CH,de;q=0.9,en;q=0.8",
	"fr-FR,fr;q=0.9,en;q=0.8",
	"fr-CA,fr;q=0.9,en;q=0.8",
	"fr-BE,fr;q=0.9,en;q=0.8",
	"es-ES,es;q=0.9,en;q=0.8",
	"es-MX,es;q=0.9,en;q=0.8",
	"es-AR,es;q=0.9,en;q=0.8",
	"pt-BR,pt;q=0.9,en;q=0.8",
	"pt-PT,pt;q=0.9,en;q=0.8",
	"it-IT,it;q=0.9,en;q=0.8",
	"nl-NL,nl;q=0.9,en;q=0.8",
	"nl-BE,nl;q=0.9,en;q=0.8",
	"pl-PL,pl;q=0.9,en;q=0.8",
	"ru-RU,ru;q=0.9,en;q=0.8",
	"ja-JP,ja;q=0.9,en;q=0.8",
	"zh-CN,zh;q=0.9,en;q=0.8",
	"zh-TW,zh;q=0.9,en;q=0.8",
	"ko-KR,ko;q=0.9,en;q=0.8",
	"ar-SA,ar;q=0.9,en;q=0.8",
	"ar-AE,ar;q=0.9,en;q=0.8",
	"tr-TR,tr;q=0.9,en;q=0.8",
	"sv-SE,sv;q=0.9,en;q=0.8",
	"nb-NO,nb;q=0.9,en;q=0.8",
	"da-DK,da;q=0.9,en;q=0.8",
	"fi-FI,fi;q=0.9,en;q=0.8",
	"zh-HK,zh;q=0.9,en;q=0.8",
	"en-IN,en;q=0.9",
	"en-SG,en;q=0.9",
	"en-NZ,en;q=0.9",
	"en-IE,en;q=0.9",
	"en-ZA,en;q=0.9",
}

func randomAcceptLanguage() string {
	return acceptLanguagePool[rand.Intn(len(acceptLanguagePool))]
}

func rotateLanguages(profile HeaderProfile) HeaderProfile {
	profile.AcceptLanguage = randomAcceptLanguage()
	return profile
}

// GetHeaderProfile returns a random header profile for the given strategy
func GetHeaderProfile(strategy HeaderStrategy) HeaderProfile {
	var profile HeaderProfile
	switch strategy {
	case StrategyModernBrowser:
		profile = modernBrowserProfiles[rand.Intn(len(modernBrowserProfiles))]
	case StrategyMobileDevice:
		profile = mobileDeviceProfiles[rand.Intn(len(mobileDeviceProfiles))]
	case StrategyBotFriendly:
		profile = botFriendlyProfiles[rand.Intn(len(botFriendlyProfiles))]
	default:
		profile = modernBrowserProfiles[0]
	}
	return rotateLanguages(profile)
}

// GetAllStrategies returns all available strategies in order of preference
func GetAllStrategies() []HeaderStrategy {
	return []HeaderStrategy{
		StrategyModernBrowser,
		StrategyMobileDevice,
		StrategyBotFriendly,
	}
}

// ToMap converts a HeaderProfile to a map of header name to value.
// Only non-empty values are included in the map.
func (p HeaderProfile) ToMap() map[string]string {
	m := make(map[string]string)
	if p.UserAgent != "" {
		m["User-Agent"] = p.UserAgent
	}
	if p.Accept != "" {
		m["Accept"] = p.Accept
	}
	if p.AcceptLanguage != "" {
		m["Accept-Language"] = p.AcceptLanguage
	}
	// NOTE: Accept-Encoding is intentionally omitted from ToMap().
	// Go's http.Client automatically adds "Accept-Encoding: gzip, deflate"
	// which is properly decompressed by the transport layer.
	// Including br/zstd here would cause decompression failures in HTTPFetcher.
	if p.SecFetchDest != "" {
		m["Sec-Fetch-Dest"] = p.SecFetchDest
	}
	if p.SecFetchMode != "" {
		m["Sec-Fetch-Mode"] = p.SecFetchMode
	}
	if p.SecFetchSite != "" {
		m["Sec-Fetch-Site"] = p.SecFetchSite
	}
	if p.SecFetchUser != "" {
		m["Sec-Fetch-User"] = p.SecFetchUser
	}
	if p.SecChUa != "" {
		m["Sec-Ch-Ua"] = p.SecChUa
	}
	if p.SecChUaMobile != "" {
		m["Sec-Ch-Ua-Mobile"] = p.SecChUaMobile
	}
	if p.SecChUaPlatform != "" {
		m["Sec-Ch-Ua-Platform"] = p.SecChUaPlatform
	}
	return m
}
