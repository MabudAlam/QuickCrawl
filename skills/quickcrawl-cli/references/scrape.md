# scrape — Fetch a single URL

Fetch a URL and extract its content in one or more formats. Supports CSS selectors, LLM-based structured extraction, and headless Chrome rendering.

## CLI

```bash
quickcrawl scrape <url> [flags]
```

### Examples

```bash
# Default: markdown output
quickcrawl scrape https://example.com

# Multiple formats
quickcrawl scrape https://example.com --formats html,markdown,links

# Extract from a specific element
quickcrawl scrape https://example.com --css-selector "article.main-content"

# Force browser rendering for JS-heavy sites
quickcrawl scrape https://example.com --render browser --wait-for 3000

# Structured extraction with JSON Schema
quickcrawl scrape https://example.com --json-schema '{"type":"object","properties":{"title":{"type":"string"}}}'
```

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--formats`, `-f` | string | `markdown` | Comma-separated: `markdown`, `html`, `rawHtml`, `plainText`, `links`, `imageLinks`, `json` |
| `--render` | string | `auto` | `auto` (HTTP first, escalate to browser), `http` (never browser), `browser` (always Chrome) |
| `--wait-for` | int | `0` | Milliseconds to wait after page load before capturing (0–120000) |
| `--include-tags` | string | `""` | Comma-separated CSS selectors to include |
| `--exclude-tags` | string | `""` | Comma-separated CSS selectors to exclude |
| `--css-selector` | string | `""` | Extract content from a specific CSS selector |
| `--json-schema` | string | `""` | JSON Schema for LLM-based structured data extraction |

### Output

```json
{
  "markdown": "# Example Domain\n\nThis domain is for...",
  "html": "<html>...",
  "plainText": "Example Domain\n\nThis domain is for...",
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

## Validation

- `url` is required and must be a valid URL
- `renderMode` must be one of: `auto`, `browser`, `http`
- `waitFor` must be 0–120000 ms
- `formats` values must be from the allowed set (unknown formats are rejected)

## Edge Cases

- **JS-heavy page (blank content)**: Use `--render browser` to force Chrome rendering
- **Paywall/welcome wall**: Use `--render browser` with a proxy (`PROXY_ENABLED=true` on CloakBrowser)
- **Slow page**: Increase `--wait-for` to give the page time to load dynamic content
- **Wrong content extracted**: Use `--css-selector` to target a specific element
- **Rate limited by site**: The CLI respects `CRAWLER__REQUESTS_PER_SECOND` in config
