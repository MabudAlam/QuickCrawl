# QuickCrawl MCP Tools — Full Reference

## Tool Summary

| Tool | Purpose | Blocking? |
|---|---|---|
| `quickcrawl_scrape` | Fetch one URL, extract content | Synchronous |
| `quickcrawl_crawl` | Start a BFS crawl of a site | Async (returns job ID immediately) |
| `quickcrawl_check_crawl_status` | Poll a crawl job for results | Synchronous (poll until done) |
| `quickcrawl_map` | Discover all URLs on a site | Synchronous |
| `quickcrawl_brand` | Extract brand identity data | Synchronous |
| `quickcrawl_search` | Search SearXNG, optionally scrape | Synchronous |

---

## quickcrawl_scrape

Scrape a single URL and return clean content in requested formats.

**Parameters:**

```json
{
  "url": "https://example.com",
  "formats": ["markdown"],
  "renderMode": "auto",
  "waitFor": 0,
  "includeTags": [],
  "excludeTags": [],
  "cssSelector": ""
}
```

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `url` | string | Yes | — | The URL to scrape |
| `formats` | string[] | No | `["markdown"]` | Output formats: `markdown`, `html`, `links`, `json` |
| `renderMode` | string | No | `"auto"` | Fetch strategy: `auto`, `browser`, `http` |
| `waitFor` | integer | No | `0` | Ms to wait after JS rendering (0–120000) |
| `includeTags` | string[] | No | `[]` | CSS selectors to include |
| `excludeTags` | string[] | No | `[]` | CSS selectors to exclude |
| `cssSelector` | string | No | `""` | Extract from specific CSS selector |

**Returns:**
```json
{
  "markdown": "# Example Domain...",
  "html": "<html>...",
  "links": ["https://iana.org/domains/example"],
  "metadata": {
    "title": "Example Domain",
    "sourceURL": "https://example.com",
    "language": "en",
    "statusCode": 200,
    "renderedMode": "http"
  }
}
```

---

## quickcrawl_crawl

Start an async BFS crawl of a website. Returns a job ID — poll `quickcrawl_check_crawl_status` until status is `completed` or `failed`.

**Parameters:**

```json
{
  "url": "https://docs.example.com",
  "maxDepth": 2,
  "maxPages": 100,
  "formats": ["markdown"],
  "renderMode": "auto",
  "waitFor": 0
}
```

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `url` | string | Yes | — | Starting URL for the crawl |
| `maxDepth` | integer | No | `2` | Max link depth (0–100) |
| `maxPages` | integer | No | `100` | Max pages to crawl (1–100) |
| `formats` | string[] | No | `["markdown"]` | Output formats per page: `markdown`, `html`, `links` |
| `renderMode` | string | No | `"auto"` | Fetch strategy per page: `auto`, `browser`, `http` |
| `waitFor` | integer | No | `0` | Ms to wait after JS rendering per page (0–120000) |

**Returns:**
```json
{ "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890", "status": "pending" }
```

---

## quickcrawl_check_crawl_status

Poll an async crawl job by its ID.

**Parameters:**

```json
{ "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890" }
```

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | string | Yes | The job ID returned from `quickcrawl_crawl` |

**Returns:**

```json
{
  "status": "running",
  "data": [
    {
      "markdown": "...",
      "metadata": { "sourceURL": "https://example.com/1", "statusCode": 200 }
    }
  ]
}
```

Status values: `pending` → `running` → `completed` | `failed`

---

## quickcrawl_map

Discover all URLs on a website via sitemap + BFS link extraction, without scraping content.

**Parameters:**

```json
{
  "url": "https://example.com",
  "maxDepth": 2,
  "useSitemap": true,
  "timeout": 30000
}
```

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `url` | string | Yes | — | Starting URL |
| `maxDepth` | integer | No | `2` | Max link depth (0–100) |
| `useSitemap` | boolean | No | `true` | Use sitemap.xml / robots.txt as seeds |
| `timeout` | integer | No | `30000` | Operation timeout in ms (1–600000) |

**Returns:**
```json
{
  "links": ["https://example.com/", "https://example.com/about"],
  "count": 42
}
```

---

## quickcrawl_brand

Extract comprehensive brand identity signals from a website: colors, fonts, logos, social links, OG metadata, and styleguide tokens. Uses headless Chrome when available for rich font and styleguide data; falls back to HTTP.

**Parameters:**

```json
{ "url": "https://example.com" }
```

| Field | Type | Required | Description |
|---|---|---|---|
| `url` | string | Yes | The URL to extract brand data from |

**Returns:**
```json
{
  "domain": "example.com",
  "title": "Example Domain",
  "name": "Example",
  "tagline": "Example tagline",
  "description": "Example description from og:description or meta",
  "primary_language": "en",
  "colors": [
    { "hex": "#ff6700", "name": "Blaze Orange" },
    { "hex": "#0a0a0a", "name": "Rich Black" }
  ],
  "logos": [
    { "url": "https://example.com/icon.svg", "format": "svg", "mode": "icon" }
  ],
  "backdrops": [
    { "url": "https://example.com/og-image.png" }
  ],
  "address": {
    "street": "123 Main St",
    "city": "San Francisco",
    "state": "CA",
    "postalCode": "94102",
    "country": "United States"
  },
  "socials": [
    { "type": "linkedin", "url": "https://linkedin.com/company/example" }
  ],
  "links": {
    "privacy": "https://example.com/privacy",
    "terms": "https://example.com/terms"
  },
  "fonts": {
    "fonts": [
      {
        "font": "Inter",
        "uses": ["h1", "h2", "p"],
        "fallbacks": ["Helvetica Neue"],
        "num_elements": 142,
        "percent_elements": 38
      }
    ],
    "fontLinks": {
      "Inter": {
        "type": "google",
        "files": { "variable": "https://fonts.gstatic.com/s/inter/v18/inter.woff2" }
      }
    }
  },
  "styleguide": {
    "mode": "light",
    "colors": {
      "accent": "#ff6700",
      "background": "#ffffff",
      "text": "#0a0a0a"
    },
    "typography": {
      "headings": {
        "h1": { "fontFamily": "Inter", "fontSize": "48px", "fontWeight": 700 }
      },
      "p": { "fontFamily": "Inter", "fontSize": "16px", "fontWeight": 400 }
    },
    "elementSpacing": { "xs": "4px", "sm": "12px", "md": "24px" },
    "shadows": { "sm": "0 1px 3px rgba(0,0,0,0.12)" },
    "components": {
      "button": {
        "primary": { "backgroundColor": "#ff6700", "color": "#ffffff", "css": "..." }
      },
      "card": { "backgroundColor": "#ffffff", "borderRadius": "8px", "css": "..." }
    }
  }
}
```

**Fields:**

| Field | Type | Description |
|---|---|---|
| `domain` | string | Domain name from URL |
| `title` | string | `<title>` or `og:title` |
| `name` | string | `og:site_name` or domain |
| `tagline` | string | `og:description` |
| `description` | string | `og:description` or `<meta name="description">` |
| `primary_language` | string | Detected language code |
| `colors` | array | CSS hex colors with human-readable names |
| `logos` | array | Favicons, apple-touch-icons, SVG icons |
| `backdrops` | array | Open Graph image URLs |
| `address` | object | Physical address from footer |
| `socials` | array | LinkedIn, X, GitHub, YouTube, Instagram, etc. |
| `links` | object | `privacy_url`, `terms_url`, `cookies_url` |
| `fonts` | object | Font usage stats + font file URLs |
| `styleguide` | object | Typography, colors, spacing, shadows, buttons, cards |

**Notes:**
- If CloakBrowser is configured, fonts and styleguide are extracted via headless Chrome
- Without a browser, HTTP-only extraction runs (fonts and styleguide will be minimal)
- Returns an error if `url` is missing or unparseable

---

## quickcrawl_search

Search SearXNG and optionally scrape result URLs in parallel with 10 concurrent workers.

**Parameters:**

```json
{
  "query": "golang web scraping",
  "region": "us-en",
  "timeRange": "week",
  "page": 1,
  "useBM25": false,
  "renderMode": "auto",
  "scrape": false,
  "formats": ["markdown"]
}
```

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `query` | string | Yes | — | The search query |
| `region` | string | No | `"us-en"` | Region code (e.g., `"us-en"`) |
| `timeRange` | string | No | `""` | Time filter: `day`, `week`, `month`, `year` |
| `page` | integer | No | `1` | Page number, 1-based (max 1000) |
| `useBM25` | boolean | No | `false` | Use BM25 scoring instead of native |
| `renderMode` | string | No | `"auto"` | Fetch strategy for scraping results |
| `scrape` | boolean | No | `false` | Also scrape each result URL |
| `formats` | string[] | No | `["markdown"]` | Output formats for scraped results |

**Returns (scrape: false):**
```json
{
  "query": "golang web scraping",
  "results": [
    {
      "position": 1,
      "score": 123.45,
      "bm25_score": 67.89,
      "site_name": "example.com",
      "snippet": "A great golang scraping library...",
      "title": "Best Web Scraping in Golang",
      "url": "https://example.com/article"
    }
  ],
  "total_results": 100,
  "page": 1
}
```

**Returns (scrape: true):** Each result also includes a `content` object with extracted data.

---

## Validation Rules

All parameters are validated server-side. Invalid values return a 400 error with a descriptive message:

| Parameter | Rule |
|---|---|
| `url` | Must be a valid, parseable URL |
| `maxDepth` | 0–100 (negative rejected) |
| `maxPages` | 1–100 (zero and negatives rejected) |
| `timeRange` | Must be one of: `day`, `week`, `month`, `year` |
| `page` | 0–1000 (0 clamped to 1) |
| `renderMode` | Must be: `auto`, `browser`, `http` |
| `waitFor` | 0–120000 ms |
| `timeout` (map) | 1–600000 ms |
| `formats` | Must be from the allowed set per tool |
| `query` | Required, non-empty |
