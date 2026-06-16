# map — Discover URLs without scraping content

Run a fast URL discovery pass on a website. Does not fetch page content — only finds and returns URLs. Uses sitemap.xml, robots.txt sitemaps, and BFS link extraction.

## CLI

```bash
quickcrawl map <url> [flags]
```

### Examples

```bash
# Default: sitemap + link discovery, up to depth 2
quickcrawl map https://example.com

# Crawl deeper
quickcrawl map https://example.com --max-depth 5

# Sitemap only (faster, no link following)
quickcrawl map https://example.com --no-sitemap

# Shorter timeout for large sites
quickcrawl map https://example.com --timeout 60000
```

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--max-depth` | int | `2` | Maximum link depth to follow (0–100) |
| `--sitemap` | bool | `true` | Use sitemap.xml and robots.txt sitemaps as seeds |
| `--timeout` | int | `30000` | Operation timeout in ms (1–600000) |

### Output

```json
{
  "links": [
    "https://example.com/",
    "https://example.com/about",
    "https://example.com/blog",
    "https://example.com/blog/post-1"
  ],
  "count": 42
}
```

## Validation

- `url` is required and must be a valid URL
- `maxDepth` must be 0–100 (negative values rejected)
- `timeout` must be 1–600000 ms

## Edge Cases

- **Very large site**: `timeout` caps the operation; increase if site is slow to crawl
- **Sitemap only**: Use `--no-sitemap` to skip sitemap discovery (faster but less complete)
- **No sitemap available**: Falls back to BFS link extraction automatically
- **Duplicate URLs**: Deduplicated before output
- **Crawl starts but times out**: Returns all URLs discovered up to the timeout point
