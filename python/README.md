# QuickCrawl Python SDK

Python SDK for QuickCrawl — a fast, configurable web scraping and crawling tool.

## Features

- **Scrape** single pages with multiple output formats (markdown, HTML, links, JSON)
- **Crawl** entire websites using BFS with configurable depth and page limits
- **Map** discover URLs without scraping content
- **Search** SearXNG and optionally scrape result pages
- JavaScript rendering support via browser backends
- Proxy configuration and stealth mode

## Installation

### From PyPI (when published)

```bash
pip install quickcrawl
```

### From Source

```bash
git clone https://github.com/MabudAlam/quickcrawl.git
cd quickcrawl/python
pip install -e .
```

### From GitHub (latest)

```bash
pip install git+https://github.com/MabudAlam/quickcrawl.git#subdirectory=python
```

## Requirements

- Python 3.9+
- `platformdirs` package (installed automatically)

## Quick Start

```python
from quickcrawl import QuickCrawlClient

# CLI mode - downloads binary automatically on first use
with QuickCrawlClient() as client:
    result = client.scrape("https://example.com")
    print(result["markdown"])
```

## Two Modes

### CLI Mode (Default)

Zero-config mode that downloads and runs the `quickcrawl` CLI binary as a subprocess. No server needed.

```python
client = QuickCrawlClient()
```

The SDK will:
1. Check if `quickcrawl` binary is in PATH
2. If not, download from GitHub releases and cache it
3. Shell out to CLI for each operation

Override binary location:
```bash
export QUICKCRAWL_BINARY=/path/to/quickcrawl
```

Or install manually:
```bash
go install github.com/MabudAlam/quickcrawl/cli
```

### HTTP Mode (Cloud/Server)

Connect to a deployed QuickCrawl server for cloud-based scraping.

```python
client = QuickCrawlClient(
    api_url="https://your-server.com",
    api_key="your-api-key"  # optional
)
```

## Usage Examples

See the `examples/` directory for complete examples:

| File | Description |
|------|-------------|
| `01_scrape.py` | Scrape a single URL |
| `02_crawl.py` | Crawl an entire website |
| `03_map.py` | Discover URLs without scraping |
| `04_formats.py` | Use multiple output formats |

### Scrape a URL

```python
from quickcrawl import QuickCrawlClient

client = QuickCrawlClient()
result = client.scrape("https://example.com", formats=["markdown", "html"])

print(result["markdown"])
print(result["html"])
```

### Crawl a Website

```python
results = client.crawl(
    "https://example.com",
    max_depth=2,
    max_pages=10,
)

for page in results:
    print(page["metadata"]["sourceURL"])
```

### Discover URLs

```python
links = client.map("https://example.com", max_depth=3)
print(f"Found {len(links)} URLs")
```

## API Reference

### `QuickCrawlClient(api_url=None, api_key=None)`

Create a client. Set `api_url` for HTTP mode, leave empty for CLI mode.

### `client.scrape(url, **kwargs)`

Scrape a single URL.

**Parameters:**
- `url` (str) — URL to scrape
- `formats` (list) — Output formats: `["markdown", "html", "links", "json"]`
- `render_mode` (str) — One of `"auto"`, `"browser"`, `"http"`. Omit to inherit server default.
- `**kwargs` — Additional options

**Returns:** Dict with keys `markdown`, `html`, `links`, `metadata`, etc.

### `client.crawl(url, max_depth=2, max_pages=10, **kwargs)`

Crawl a website.

**Parameters:**
- `url` (str) — Starting URL
- `max_depth` (int) — Maximum link depth (default: 2)
- `max_pages` (int) — Maximum pages to scrape (default: 10)

**Returns:** List of page result dicts.

### `client.map(url, max_depth=2, use_sitemap=True)`

Discover URLs without scraping content.

**Parameters:**
- `url` (str) — Starting URL
- `max_depth` (int) — Maximum link depth
- `use_sitemap` (bool) — Use sitemap.xml as seeds

**Returns:** List of discovered URLs.

## Development

Run tests:
```bash
pip install -e ".[test]"
pytest
```

## License

MIT
