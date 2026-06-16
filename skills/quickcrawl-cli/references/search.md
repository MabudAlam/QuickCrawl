# search — Web search via SearXNG

Search the web using SearXNG. Returns titles, URLs, snippets, and optionally scraped content from each result. Uses 10 concurrent workers to scrape results when `--scrape` / `scrape: true` is set.

## CLI

```bash
quickcrawl search "<query>" [flags]
```

### Examples

```bash
# Basic search
quickcrawl search "golang web scraping frameworks"

# Search with time filter
quickcrawl search "golang" --time-range week

# Search and scrape each result
quickcrawl search "golang" --scrape --formats markdown

# Multi-page search
quickcrawl search "golang" --page 2

# BM25 scoring for relevance
quickcrawl search "golang" --use-bm25

# Regional search
quickcrawl search "golang" --region us-en
```

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--formats` | string | `markdown` | Comma-separated: `markdown`, `html`, `rawHtml`, `plainText`, `links`, `imageLinks`, `json` |
| `--region` | string | `us-en` | Region code (e.g., `us-en`, `gb-en`, `de-de`) |
| `--time-range` | string | `""` | SearXNG time filter: `day`, `week`, `month`, `year`. Omit for no filter |
| `--page` | int | `1` | Page number (1-indexed, max 1000) |
| `--render-mode` | string | `auto` | Render mode for scraping each result: `auto`, `browser`, `http` |
| `--scrape` | bool | `false` | Also scrape content from each result URL |
| `--use-bm25` | bool | `false` | Re-rank results using BM25 scoring |
| `--workers` | int | `10` | Concurrent workers for scraping results |

### Output (no scrape)

```json
{
  "query": "golang web scraping",
  "results": [
    {
      "position": 1,
      "score": 123.45,
      "site_name": "example.com",
      "snippet": "A great golang web scraping library...",
      "title": "Best Web Scraping Libraries in Golang",
      "url": "https://example.com/article"
    }
  ],
  "total_results": 100,
  "page": 1
}
```

### Output (with --scrape)

When `--scrape` is enabled, each result includes extracted content:

```json
{
  "query": "golang",
  "results": [
    {
      "position": 1,
      "score": 123.45,
      "site_name": "example.com",
      "snippet": "A great golang...",
      "title": "Best Web Scraping in Golang",
      "url": "https://example.com/article",
      "content": {
        "markdown": "# Best Web Scraping...\n\nContent here...",
        "metadata": { "title": "Best Web Scraping in Golang" }
      }
    }
  ],
  "total_results": 100,
  "page": 1
}
```

## Validation

- `query` is required and cannot be empty
- `timeRange` must be one of: `day`, `week`, `month`, `year`
- `page` must be 0–1000 (0 is clamped to 1)
- `renderMode` must be `auto`, `browser`, or `http`
- `formats` values must be from the allowed set

## Edge Cases

- **No results**: Check that `SEARCH__BASE_URL` is correctly configured (SearXNG must be reachable)
- **Stale results**: Try with `--time-range day` to get recent results only
- **Too many scrape failures**: Reduce `--workers` to lower concurrency
- **Rate limiting from SearXNG**: The server has its own rate limits; back off if seeing 429s
- **Scraping blocked by result sites**: Use `--render-mode browser` with a proxy configured on CloakBrowser
