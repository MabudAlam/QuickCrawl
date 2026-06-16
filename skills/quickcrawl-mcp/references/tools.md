# QuickCrawl MCP Tools — Full Reference

## Tool Summary

| Tool | Purpose | Blocking? |
|---|---|---|
| `quickcrawl_scrape` | Fetch one URL, extract content | Synchronous |
| `quickcrawl_crawl` | Start a BFS crawl of a site | Async (returns job ID immediately) |
| `quickcrawl_check_crawl_status` | Poll a crawl job for results | Synchronous (poll until done) |
| `quickcrawl_map` | Discover all URLs on a site | Synchronous |
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
