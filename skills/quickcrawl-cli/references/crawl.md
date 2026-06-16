# crawl — Async BFS site crawl

Start a breadth-first crawl of a website. Returns a job ID — poll with `quickcrawl check-crawl-status <job-id>` for results. The crawl runs asynchronously; the first call starts the job and returns immediately.

## CLI

```bash
quickcrawl crawl <url> [flags]
```

### Examples

```bash
# Default: crawl up to 100 pages, markdown output
quickcrawl crawl https://example.com

# Crawl docs site, up to 50 pages
quickcrawl crawl https://docs.example.com --max-pages 50 --max-depth 3

# HTML output for all pages
quickcrawl crawl https://example.com --formats html

# Force browser rendering for JS-heavy sites
quickcrawl crawl https://example.com --render browser --wait-for 2000

# Verbose progress (prints every 2s)
quickcrawl crawl https://example.com --max-pages 20 -v
```

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--formats`, `-f` | string | `markdown` | Comma-separated: `markdown`, `html`, `rawHtml`, `plainText`, `links`, `imageLinks`. **Note:** `json` is not supported on crawl |
| `--render` | string | `auto` | `auto`, `http`, `browser` |
| `--wait-for` | int | `0` | Milliseconds to wait after page load per page (0–120000) |
| `--max-depth` | int | `2` | Maximum link depth to follow (0–100) |
| `--max-pages` | int | `100` | Maximum number of pages to crawl (1–100) |

### Output

Each page outputs one JSON object to stdout (one per line):

```json
{
  "markdown": "# Example Page\n\nContent here...",
  "html": "<html>...",
  "links": ["https://example.com/page1", "https://example.com/page2"],
  "metadata": {
    "title": "Example Page",
    "sourceURL": "https://example.com/page1",
    "statusCode": 200,
    "renderedMode": "http"
  }
}
```

## Checking crawl status

```bash
quickcrawl check-crawl-status <job-id>
```

**Returns:**
```json
{
  "status": "running",
  "data": [
    { "markdown": "...", "metadata": { "sourceURL": "https://example.com/1" } },
    { "markdown": "...", "metadata": { "sourceURL": "https://example.com/2" } }
  ]
}
```

Status values: `pending` → `running` → `completed` | `failed`

## Validation

- `url` is required and must be a valid URL
- `maxDepth` must be 0–100 (negative values rejected)
- `maxPages` must be 1–100 (zero and negatives rejected)
- `renderMode` must be `auto`, `browser`, or `http`
- `waitFor` must be 0–120000 ms
- `json` format is not supported on crawl (use `quickcrawl scrape` with `--json-schema` for LLM extraction)

## Edge Cases

- **Crawl not completing**: The job has a 1-hour TTL; poll regularly
- **Too many pages**: Use `--max-pages` to cap scope; use `quickcrawl map` first to estimate size
- **JS-heavy pages**: Use `--render browser` (requires CloakBrowser configured)
- **Rate limited**: Increase `CRAWLER__REQUESTS_PER_SECOND` in config (may be blocked)
- **Duplicate URLs**: The crawler deduplicates by URL; same page at different depths still counted once
