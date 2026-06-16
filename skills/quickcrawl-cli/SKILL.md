---
name: quickcrawl-cli
description: "Scrape URLs, crawl sites, map pages, and search the web using the QuickCrawl CLI. Use when the agent has shell access and needs to fetch web content, discover site structure, or run web searches. The `quickcrawl` binary runs standalone with no server required."
license: AGPL-3.0
metadata:
  author: MabudAlam
  version: "0.3.0"
  homepage: https://github.com/MabudAlam/quickcrawl
allowed-tools: Bash(quickcrawl:*) Read
---

# QuickCrawl CLI — Web Data Toolkit for AI Agents

## When to use this skill

Use this skill when:
- The agent has shell access and needs to fetch a web page
- The agent needs to extract content from a URL for context or research
- The agent needs to crawl an entire website and discover all its pages
- The agent needs to search the web and get results with optional page scraping

## Installation

### Install the `quickcrawl` binary

```bash
# One-line install (recommended)
curl -fsSL https://raw.githubusercontent.com/MabudAlam/quickcrawl/main/install.sh | sh

# Or from source
go install github.com/MabudAlam/quickcrawl/cli
```

### Verify it's working

```bash
quickcrawl --help
```

### Install this skill for the agent

From the repo root, copy the skill directory to your agent's skills directory:

```bash
cp -r skills/quickcrawl-cli ~/.claude/skills/quickcrawl-cli
```

## Tools Reference

| Command | What it does | Async? |
|---|---|---|
| `quickcrawl scrape` | Fetch a single URL, extract content in any format | No |
| `quickcrawl crawl` | BFS crawl a site, returns a job ID to poll | Yes |
| `quickcrawl check-crawl-status <id>` | Poll a crawl job for results | No |
| `quickcrawl map` | Discover all URLs on a site without scraping content | No |
| `quickcrawl search` | Search SearXNG, optionally scrape result pages | No |

## Quick Examples

```bash
# Scrape a page for context
quickcrawl scrape https://example.com

# Crawl a docs site
JOB=$(quickcrawl crawl https://docs.example.com --max-pages 100 | jq -r .id)
quickcrawl check-crawl-status "$JOB"

# Discover all URLs before crawling
quickcrawl map https://example.com

# Search the web and get results
quickcrawl search "golang web scraping frameworks"

# Search and scrape top results
quickcrawl search "golang" --scrape --page 1
```

## Detailed Reference

- [scrape](skills/quickcrawl-cli/references/scrape.md) — all flags, formats, CSS selectors, JSON schema
- [crawl](skills/quickcrawl-cli/references/crawl.md) — maxDepth, maxPages, async job flow, output format
- [map](skills/quickcrawl-cli/references/map.md) — sitemap discovery, timeout, output format
- [search](skills/quickcrawl-cli/references/search.md) — timeRange, page, region, scraping results

## Validation Rules

Know these before calling — invalid values return errors:

| Parameter | Commands | Allowed values |
|---|---|---|
| `--max-depth` | crawl, map | `0`–`100` (negative rejected) |
| `--max-pages` | crawl | `1`–`100` (zero/negative rejected) |
| `--wait-for` | scrape, crawl | `0`–`120000` ms |
| `--timeout` | map | `1`–`600000` ms (default 30000) |
| `--time-range` | search | `day`, `week`, `month`, `year` |
| `--page` | search | `1`–`1000` (1-indexed) |
| `--render` | scrape, crawl | `auto`, `http`, `browser` |
| `--formats` | scrape, crawl, search | Allowlist per command (`json` rejected on crawl) |
| `query` | search | Required, non-empty |
| `url` | scrape, crawl, map | Required, must be valid URL |

## Output

All commands output **JSON** to stdout. Logs go to stderr (silent in production builds).

```bash
# Pipe through jq for field extraction
quickcrawl scrape https://example.com | jq '.markdown'

# Save to file
quickcrawl crawl https://docs.example.com --max-pages 50 > crawl-output.json

# Extract just the markdown content
quickcrawl scrape https://example.com 2>/dev/null | jq -r '.markdown // .metadata.title'
```

> **Content bounds:** Scraped content can be large (thousands of lines). If the agent has a context limit, pipe through `head` or use `jq` to extract only the needed fields.

## Configuration

QuickCrawl reads `quickcrawl.toml` in the same directory as the binary. Key overrides:

```bash
# Point at a local SearXNG instance
SEARCH__BASE_URL=http://localhost:8888/ quickcrawl search "query"

# Point at a local CloakBrowser
RENDERER__CHROME__WS_URL=ws://localhost:9222/ quickcrawl scrape https://example.com --render browser

# Bump crawl concurrency
CRAWLER__MAX_CONCURRENCY=80 quickcrawl crawl https://example.com
```

## Common Edge Cases

- **JavaScript-heavy sites**: Use `--render browser` to force headless Chrome rendering
- **Rate limiting**: Increase `CRAWLER__REQUESTS_PER_SECOND` for faster crawling (may be blocked by sites)
- **Large sites**: Use `quickcrawl map` first to see how many URLs exist before crawling
- **Empty results**: If `markdown` is empty, try `--formats html` or `--formats rawHtml`
- **Timeout**: Crawl and map have a 60s server timeout; large sites may need `--timeout` override

## Links

- GitHub: https://github.com/MabudAlam/quickcrawl
- Full docs: https://github.com/MabudAlam/quickcrawl#readme
- Report issues: https://github.com/MabudAlam/quickcrawl/issues
